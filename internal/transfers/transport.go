// Package transfers implements application-layer encryption for passive tunnel
// observers. It does not authenticate JavaScript delivered through that tunnel.
package transfers

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

const (
	BlockSize    = 4 << 20
	maxMessages  = 1000000
	maxTransfers = 64
	lifetime     = 15 * time.Minute
	prefix       = "/api/v1/crypto/"
)

var errInvalid = errors.New("invalid encrypted transfer")

// Manager holds ephemeral keys and bounded transfer state. Close removes its
// private staging directory. Neither keys nor staging files are served by HTTP.
type Manager struct {
	key         *rsa.PrivateKey
	public      string
	fingerprint string
	dir         string
	next        http.Handler
	maxUpload   int64
	mu          sync.Mutex
	items       map[string]*transfer
	seen        map[string]time.Time
	stop        chan struct{}
	done        chan struct{}
	closeOnce   sync.Once
}
type transfer struct {
	mu                sync.Mutex
	request, response cipher.AEAD
	meta              requestMetadata
	file              *os.File
	frames            []frame
	received          int64
	next              int
	ready, dead       bool
	touched           time.Time
}
type requestMetadata struct {
	Method   string            `json:"method"`
	URL      string            `json:"url"`
	Headers  map[string]string `json:"headers"`
	IssuedAt int64             `json:"issuedAt"`
}
type frame struct {
	offset int64
	length int
}
type responseMetadata struct {
	Status  int         `json:"status"`
	Headers http.Header `json:"headers"`
	Size    int64       `json:"size"`
	Chunks  int         `json:"chunks"`
}

func New(next http.Handler, maxUpload int64) (*Manager, error) {
	key, err := rsa.GenerateKey(rand.Reader, 3072)
	if err != nil {
		return nil, err
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "upgopher-transfers-*")
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(der)
	m := &Manager{key: key, public: base64.StdEncoding.EncodeToString(der), fingerprint: hex.EncodeToString(digest[:]), dir: dir, next: next, maxUpload: maxUpload, items: make(map[string]*transfer), seen: make(map[string]time.Time), stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(m.done)
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case now := <-ticker.C:
				m.expire(now)
			case <-m.stop:
				return
			}
		}
	}()
	return m, nil
}
func (m *Manager) Fingerprint() string { return m.fingerprint }
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		close(m.stop)
		<-m.done
		m.mu.Lock()
		defer m.mu.Unlock()
		for id, t := range m.items {
			t.mu.Lock()
			m.removeLocked(id, t)
			t.mu.Unlock()
		}
		_ = os.RemoveAll(m.dir)
	})
}
func (m *Manager) expire(now time.Time) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, t := range m.items {
		// An in-flight request owns t.mu; expiry never races its files or keys.
		if !t.mu.TryLock() {
			continue
		}
		if now.Sub(t.touched) >= lifetime {
			m.removeLocked(id, t)
		}
		t.mu.Unlock()
	}
	for id, until := range m.seen {
		if !now.Before(until) {
			delete(m.seen, id)
		}
	}
}
func (m *Manager) removeLocked(id string, t *transfer) {
	t.dead = true
	if t.file != nil {
		_ = t.file.Close()
		_ = os.Remove(t.file.Name())
		t.file = nil
	}
	t.request = nil
	t.response = nil
	delete(m.items, id)
}
func aad(id, kind string, index int) []byte {
	return []byte("upgopher-v1:" + id + ":" + kind + ":" + strconv.Itoa(index))
}
func sealPacket(a cipher.AEAD, id, kind string, index int, plain []byte) ([]byte, error) {
	if index < 0 || index >= maxMessages {
		return nil, errInvalid
	}
	nonce := make([]byte, a.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return a.Seal(nonce, nonce, plain, aad(id, kind, index)), nil
}
func openPacket(a cipher.AEAD, id, kind string, index int, packet []byte) ([]byte, error) {
	if index < 0 || index >= maxMessages || len(packet) < a.NonceSize()+a.Overhead() {
		return nil, errInvalid
	}
	return a.Open(nil, packet[:a.NonceSize()], packet[a.NonceSize():], aad(id, kind, index))
}
func newAEAD(key []byte) (cipher.AEAD, error) {
	b, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(b)
}
func readLimited(w http.ResponseWriter, r *http.Request, size int64) ([]byte, error) {
	r.Body = http.MaxBytesReader(w, r.Body, size)
	return io.ReadAll(r.Body)
}
func failure(w http.ResponseWriter, status int) { http.Error(w, "Encrypted transfer failed", status) }
func writePacket(w http.ResponseWriter, packet []byte) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	_, _ = w.Write(packet)
}
func (m *Manager) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	// Browser requests are same-origin. Refuse cross-origin initiation, including
	// requests with credentials; no permissive CORS headers are issued here.
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.Host != r.Host || (u.Scheme != "http" && u.Scheme != "https") {
			failure(w, http.StatusForbidden)
			return
		}
	}
	switch r.URL.Path {
	case prefix + "key":
		if r.Method != http.MethodGet {
			failure(w, 405)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{"version": 1, "spki": m.public, "fingerprint": m.fingerprint, "serverTime": time.Now().UnixMilli(), "blockSize": BlockSize})
	case prefix + "transfers":
		if r.Method != http.MethodPost {
			failure(w, 405)
			return
		}
		m.begin(w, r)
	default:
		suffix := strings.TrimPrefix(r.URL.Path, prefix+"transfers/")
		parts := strings.Split(suffix, "/")
		if len(parts) < 1 || len(parts) > 2 || len(parts[0]) != 32 {
			failure(w, 404)
			return
		}
		id := parts[0]
		// Lock order is always manager -> transfer. Ordinary work releases the
		// manager lock before touching disk or invoking application handlers.
		m.mu.Lock()
		t := m.items[id]
		if t == nil {
			m.mu.Unlock()
			failure(w, 410)
			return
		}
		t.mu.Lock()
		m.mu.Unlock()
		defer func() {
			dead := t.dead
			t.mu.Unlock()
			if dead {
				m.mu.Lock()
				t.mu.Lock()
				if m.items[id] == t {
					m.removeLocked(id, t)
				}
				t.mu.Unlock()
				m.mu.Unlock()
			}
		}()
		if t.dead || time.Since(t.touched) >= lifetime {
			failure(w, 410)
			return
		}
		t.touched = time.Now()
		if len(parts) == 1 && r.Method == http.MethodDelete {
			// Keep a tombstone until expiry; it cannot be reinitialized or used again.
			t.dead = true
			if t.file != nil {
				_ = t.file.Close()
				_ = os.Remove(t.file.Name())
				t.file = nil
			}
			t.request = nil
			t.response = nil
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if len(parts) != 2 {
			failure(w, 405)
			return
		}
		switch parts[1] {
		case "chunks":
			m.chunk(w, r, id, t)
		case "completion":
			m.complete(w, r, id, t)
		default:
			failure(w, 404)
		}
	}
}
func (m *Manager) begin(w http.ResponseWriter, r *http.Request) {
	data, err := readLimited(w, r, 64<<10)
	if err != nil {
		failure(w, 413)
		return
	}
	var req struct {
		ID       string `json:"id"`
		Key      string `json:"key"`
		Metadata string `json:"metadata"`
	}
	if json.Unmarshal(data, &req) != nil || len(req.ID) != 32 {
		failure(w, 400)
		return
	}
	if decoded, err := hex.DecodeString(req.ID); err != nil || len(decoded) != 16 {
		failure(w, 400)
		return
	}
	wrapped, err := base64.StdEncoding.DecodeString(req.Key)
	if err != nil {
		failure(w, 400)
		return
	}
	keys, err := rsa.DecryptOAEP(sha256.New(), rand.Reader, m.key, wrapped, []byte("upgopher-transfer-v1"))
	if err != nil || len(keys) != 64 {
		failure(w, 400)
		return
	}
	up, err := newAEAD(keys[:32])
	if err != nil {
		failure(w, http.StatusInternalServerError)
		return
	}
	down, err := newAEAD(keys[32:])
	if err != nil {
		failure(w, http.StatusInternalServerError)
		return
	}
	for i := range keys {
		keys[i] = 0
	}
	packet, err := base64.StdEncoding.DecodeString(req.Metadata)
	if err != nil {
		failure(w, 400)
		return
	}
	plain, err := openPacket(up, req.ID, "request-metadata", 0, packet)
	if err != nil {
		failure(w, 400)
		return
	}
	var meta requestMetadata
	if json.Unmarshal(plain, &meta) != nil || !validRequest(meta) {
		failure(w, 400)
		return
	}
	issued := time.UnixMilli(meta.IssuedAt)
	now := time.Now()
	if issued.After(now.Add(30*time.Second)) || now.Sub(issued) > lifetime {
		failure(w, 400)
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	for id, until := range m.seen {
		if !now.Before(until) {
			delete(m.seen, id)
		}
	}
	if _, ok := m.seen[req.ID]; ok {
		failure(w, 409)
		return
	}
	if len(m.items) >= maxTransfers || len(m.seen) >= 4096 {
		failure(w, 429)
		return
	}
	f, err := os.CreateTemp(m.dir, "request-*")
	if err != nil {
		failure(w, 500)
		return
	}
	safe, err := security.IsSafePath(m.dir, f.Name())
	if err != nil || !safe {
		_ = f.Close()
		_ = os.Remove(f.Name())
		failure(w, 500)
		return
	}
	m.items[req.ID] = &transfer{request: up, response: down, meta: meta, file: f, touched: now}
	m.seen[req.ID] = issued.Add(lifetime + 30*time.Second)
	w.WriteHeader(http.StatusCreated)
}
func validRequest(meta requestMetadata) bool {
	if meta.Method != http.MethodGet && meta.Method != http.MethodPost && meta.Method != http.MethodDelete {
		return false
	}
	u, err := url.ParseRequestURI(meta.URL)
	if err != nil || u.IsAbs() || u.Host != "" || !strings.HasPrefix(u.Path, "/") || strings.HasPrefix(u.Path, "//") || strings.ContainsAny(meta.URL, "\r\n\\") {
		return false
	}
	if !Sensitive(u.Path, meta.Method) {
		return false
	}
	for key, value := range meta.Headers {
		if strings.ContainsAny(value, "\r\n") {
			return false
		}
		switch http.CanonicalHeaderKey(key) {
		case "Content-Type", "Accept", "X-Requested-With":
		default:
			return false
		}
	}
	return true
}
func (m *Manager) chunk(w http.ResponseWriter, r *http.Request, id string, t *transfer) {
	index, err := strconv.Atoi(r.URL.Query().Get("index"))
	if err != nil || index < 0 || index >= maxMessages-2 {
		failure(w, 400)
		return
	}
	if r.Method == http.MethodPost && !t.ready {
		if index != t.next {
			failure(w, 409)
			return
		}
		packet, err := readLimited(w, r, BlockSize+28)
		if err != nil {
			failure(w, 413)
			return
		}
		plain, err := openPacket(t.request, id, "upload", index, packet)
		if err != nil || len(plain) == 0 {
			failure(w, 400)
			return
		}
		if m.maxUpload > 0 && int64(len(plain)) > m.maxUpload-t.received {
			failure(w, 413)
			return
		}
		if _, err = t.file.Write(plain); err != nil {
			failure(w, 500)
			return
		}
		t.received += int64(len(plain))
		t.next++
		w.WriteHeader(http.StatusNoContent)
		return
	}
	if r.Method == http.MethodGet && t.ready {
		if index >= len(t.frames) {
			failure(w, 400)
			return
		}
		f := t.frames[index]
		data := make([]byte, f.length)
		if _, err = t.file.ReadAt(data, f.offset); err != nil {
			failure(w, 500)
			return
		}
		// Return the original ciphertext on retries; never encrypt changed data
		// under a reused nonce. Indices and transfer identity are authenticated.
		writePacket(w, data)
		return
	}
	failure(w, 409)
}
func (m *Manager) complete(w http.ResponseWriter, r *http.Request, id string, t *transfer) {
	if r.Method != http.MethodPost || t.ready {
		failure(w, 409)
		return
	}
	packet, err := readLimited(w, r, 4096)
	if err != nil {
		failure(w, 413)
		return
	}
	plain, err := openPacket(t.request, id, "finish", t.next, packet)
	if err != nil {
		failure(w, 400)
		return
	}
	var finish struct {
		Size   int64 `json:"size"`
		Chunks int   `json:"chunks"`
	}
	if json.Unmarshal(plain, &finish) != nil || finish.Size != t.received || finish.Chunks != t.next {
		failure(w, 400)
		return
	}
	if _, err = t.file.Seek(0, io.SeekStart); err != nil {
		failure(w, 500)
		return
	}
	inner, err := http.NewRequestWithContext(r.Context(), t.meta.Method, t.meta.URL, t.file)
	if err != nil {
		failure(w, 400)
		return
	}
	inner.ContentLength = t.received
	inner.Host = r.Host
	inner.RemoteAddr = r.RemoteAddr
	for key, value := range t.meta.Headers {
		inner.Header.Set(key, value)
	}
	// Authentication belongs to the outer request, never to encrypted user-
	// supplied metadata. Current handlers receive its existing credentials.
	inner.Header.Set("Authorization", r.Header.Get("Authorization"))
	out, err := os.CreateTemp(m.dir, "response-*")
	if err != nil {
		failure(w, 500)
		return
	}
	safe, err := security.IsSafePath(m.dir, out.Name())
	if err != nil || !safe {
		_ = out.Close()
		_ = os.Remove(out.Name())
		failure(w, 500)
		return
	}
	published := false
	defer func() {
		if !published {
			_ = out.Close()
			_ = os.Remove(out.Name())
		}
	}()
	cw := &encryptedWriter{header: make(http.Header), file: out, aead: t.response, id: id}
	m.next.ServeHTTP(cw, inner)
	if err = cw.finish(); err != nil {
		_ = out.Close()
		_ = os.Remove(out.Name())
		failure(w, 500)
		return
	}
	_ = t.file.Close()
	_ = os.Remove(t.file.Name())
	t.file = out
	published = true
	t.frames = cw.frames
	t.ready = true
	meta, err := json.Marshal(responseMetadata{Status: cw.status, Headers: cw.header, Size: cw.size, Chunks: len(cw.frames)})
	if err != nil {
		failure(w, 500)
		return
	}
	answer, err := sealPacket(t.response, id, "response-metadata", 0, meta)
	if err != nil {
		failure(w, 500)
		return
	}
	writePacket(w, answer)
}

type encryptedWriter struct {
	header http.Header
	status int
	file   *os.File
	aead   cipher.AEAD
	id     string
	buffer []byte
	frames []frame
	size   int64
	offset int64
	err    error
}

func (w *encryptedWriter) Header() http.Header { return w.header }
func (w *encryptedWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *encryptedWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.err != nil {
		return 0, w.err
	}
	count := len(p)
	for len(p) > 0 {
		n := BlockSize - len(w.buffer)
		if n > len(p) {
			n = len(p)
		}
		w.buffer = append(w.buffer, p[:n]...)
		p = p[n:]
		w.size += int64(n)
		if len(w.buffer) == BlockSize {
			if err := w.flushBlock(); err != nil {
				return count - len(p), err
			}
		}
	}
	return count, nil
}
func (w *encryptedWriter) flushBlock() error {
	if len(w.frames) >= maxMessages-2 {
		w.err = errInvalid
		return w.err
	}
	if w.err != nil {
		return w.err
	}
	packet, err := sealPacket(w.aead, w.id, "response-body", len(w.frames), w.buffer)
	if err == nil {
		_, err = w.file.Write(packet)
		if err == nil {
			w.frames = append(w.frames, frame{w.offset, len(packet)})
			w.offset += int64(len(packet))
		}
	}
	w.err = err
	w.buffer = w.buffer[:0]
	return err
}
func (w *encryptedWriter) finish() error {
	if w.status == 0 {
		w.status = 200
	}
	if len(w.buffer) > 0 {
		return w.flushBlock()
	}
	return w.err
}

// Sensitive enumerates every route that can send file content, accept a file,
// or return excerpts. Unknown paths are custom aliases handled by the root.
func Sensitive(path, method string) bool {
	switch path {
	case "/":
		return method != http.MethodGet && method != http.MethodHead
	case "/zip", "/zip-selected", "/file-content", "/search-file", "/api/v1/screenshots/image":
		return true
	case "/api/v1/screenshots":
		return method == http.MethodPost
	case "/api/v1/breadcrumbs", "/api/v1/tree", "/clipboard", "/clipboard/tabs", "/clipboard/stream", "/mkdir", "/custom-path", "/showhiddenfiles", "/favicon.ico", "/static/logopher.webp":
		return false
	}
	if strings.HasPrefix(path, prefix) {
		return false
	}
	if strings.HasPrefix(path, "/delete/") {
		return false
	}
	return true
}

// Guard prevents plaintext file delivery even with valid access credentials.
// Navigating to a shared link returns a browser loader, never the file itself.
func (m *Manager) Guard(loader func(http.ResponseWriter, *http.Request)) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, prefix) {
			m.ServeHTTP(w, r)
			return
		}
		if Sensitive(r.URL.Path, r.Method) {
			if r.Method == http.MethodGet && strings.Contains(r.Header.Get("Accept"), "text/html") {
				loader(w, r)
				return
			}
			failure(w, http.StatusUpgradeRequired)
			return
		}
		m.next.ServeHTTP(w, r)
	})
}
