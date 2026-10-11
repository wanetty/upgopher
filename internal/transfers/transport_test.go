package transfers

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/wanetty/upgopher/internal/handlers"
	"github.com/wanetty/upgopher/internal/security"
)

func TestAuthenticatedPackets(t *testing.T) {
	b, _ := aes.NewCipher(make([]byte, 32))
	aead, _ := cipher.NewGCM(b)
	packet, err := sealPacket(aead, "transfer", "upload", 0, []byte("private file"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := openPacket(aead, "transfer", "upload", 0, packet)
	if err != nil || string(plain) != "private file" {
		t.Fatalf("round trip: %q, %v", plain, err)
	}
	for _, tc := range []struct {
		name, id, kind string
		index          int
	}{
		{"other transfer", "other", "upload", 0},
		{"other operation", "transfer", "finish", 0},
		{"other index", "transfer", "upload", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := openPacket(aead, tc.id, tc.kind, tc.index, packet); err == nil {
				t.Fatal("accepted substituted packet")
			}
		})
	}
	packet[len(packet)-1] ^= 1
	if _, err := openPacket(aead, "transfer", "upload", 0, packet); err == nil {
		t.Fatal("accepted corrupted packet")
	}
}

// These tests exercise the HTTP boundary without listening on a network port.
type testClient struct {
	t        *testing.T
	m        *Manager
	id       string
	up, down cipher.AEAD
	initial  []byte
	count    int
	size     int64
}

func newClient(t *testing.T, m *Manager, method, url string, headers map[string]string) *testClient {
	t.Helper()
	keys := make([]byte, 64)
	_, _ = rand.Read(keys)
	idBytes := make([]byte, 16)
	_, _ = rand.Read(idBytes)
	c := &testClient{t: t, m: m, id: hex.EncodeToString(idBytes)}
	c.up, _ = newAEAD(keys[:32])
	c.down, _ = newAEAD(keys[32:])
	wrapped, err := rsa.EncryptOAEP(sha256.New(), rand.Reader, &m.key.PublicKey, keys, []byte("upgopher-transfer-v1"))
	if err != nil {
		t.Fatal(err)
	}
	meta, _ := json.Marshal(requestMetadata{method, url, headers, time.Now().UnixMilli()})
	packet, _ := sealPacket(c.up, c.id, "request-metadata", 0, meta)
	body, _ := json.Marshal(map[string]string{"id": c.id, "key": base64.StdEncoding.EncodeToString(wrapped), "metadata": base64.StdEncoding.EncodeToString(packet)})
	c.initial = body
	response := c.call("POST", prefix+"transfers", body)
	if response.Code != 201 {
		t.Fatalf("init: %d %s", response.Code, response.Body.String())
	}
	return c
}
func (c *testClient) call(method, path string, body []byte) *httptest.ResponseRecorder {
	c.t.Helper()
	w := httptest.NewRecorder()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	c.m.ServeHTTP(w, r)
	return w
}
func (c *testClient) upload(body []byte) {
	c.t.Helper()
	for len(body) > 0 {
		n := BlockSize
		if n > len(body) {
			n = len(body)
		}
		packet, _ := sealPacket(c.up, c.id, "upload", c.count, body[:n])
		w := c.call("POST", prefix+"transfers/"+c.id+"/chunks?index="+strconv.Itoa(c.count), packet)
		if w.Code != 204 {
			c.t.Fatalf("upload: %d %s", w.Code, w.Body.String())
		}
		c.count++
		c.size += int64(n)
		body = body[n:]
	}
}
func (c *testClient) complete() (responseMetadata, []byte) {
	c.t.Helper()
	finish, _ := json.Marshal(map[string]interface{}{"size": c.size, "chunks": c.count})
	packet, _ := sealPacket(c.up, c.id, "finish", c.count, finish)
	w := c.call("POST", prefix+"transfers/"+c.id+"/completion", packet)
	if w.Code != 200 {
		c.t.Fatalf("complete: %d %s", w.Code, w.Body.String())
	}
	plain, err := openPacket(c.down, c.id, "response-metadata", 0, w.Body.Bytes())
	if err != nil {
		c.t.Fatal(err)
	}
	var meta responseMetadata
	if err = json.Unmarshal(plain, &meta); err != nil {
		c.t.Fatal(err)
	}
	var body []byte
	for i := 0; i < meta.Chunks; i++ {
		w = c.call("GET", prefix+"transfers/"+c.id+"/chunks?index="+strconv.Itoa(i), nil)
		if w.Code != 200 {
			c.t.Fatal(w.Code)
		}
		block, err := openPacket(c.down, c.id, "response-body", i, w.Body.Bytes())
		if err != nil {
			c.t.Fatal(err)
		}
		if bytes.Contains(w.Body.Bytes(), []byte("private content")) {
			c.t.Fatal("plaintext on wire")
		}
		body = append(body, block...)
	}
	if int64(len(body)) != meta.Size {
		c.t.Fatal("length mismatch")
	}
	c.call("DELETE", prefix+"transfers/"+c.id, nil)
	return meta, body
}
func newTestManager(t *testing.T, h http.Handler, limit int64) *Manager {
	t.Helper()
	m, err := New(h, limit)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.Close)
	return m
}
func TestEncryptedHTTPRoundTrips(t *testing.T) {
	m := newTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = io.Copy(w, r.Body)
	}), 0)
	for _, size := range []int{0, 1, BlockSize - 1, BlockSize, BlockSize + 1, 2*BlockSize + 37} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			body := make([]byte, size)
			_, _ = rand.Read(body)
			c := newClient(t, m, "POST", "/", map[string]string{"Content-Type": "application/octet-stream"})
			c.upload(body)
			meta, got := c.complete()
			if meta.Status != 200 || !bytes.Equal(got, body) {
				t.Fatal("file differs after round trip")
			}
		})
	}
	if len(m.items) != 0 {
		t.Fatal("finished transfers still occupy active slots")
	}
}
func TestRejectInvalidTransferBlocks(t *testing.T) {
	m := newTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Fatal("dispatched invalid request") }), 16)
	for _, name := range []string{"changed", "reordered", "too large", "missing", "repeated", "expired"} {
		t.Run(name, func(t *testing.T) {
			c := newClient(t, m, "POST", "/", nil)
			packet, _ := sealPacket(c.up, c.id, "upload", 0, []byte("private content"))
			path := prefix + "transfers/" + c.id + "/chunks?index=0"
			want := 400
			switch name {
			case "changed":
				packet[len(packet)-1] ^= 1
			case "reordered":
				path = prefix + "transfers/" + c.id + "/chunks?index=1"
				want = 409
			case "too large":
				packet, _ = sealPacket(c.up, c.id, "upload", 0, make([]byte, 17))
				want = 413
			case "missing":
				packet, _ = sealPacket(c.up, c.id, "finish", 0, []byte(`{"size":15,"chunks":1}`))
				path = prefix + "transfers/" + c.id + "/completion"
			case "repeated":
				c.upload([]byte("private content"))
				want = 409
			case "expired":
				m.items[c.id].touched = time.Now().Add(-lifetime)
				want = 410
			}
			w := c.call("POST", path, packet)
			if w.Code != want {
				t.Fatalf("got %d, want %d", w.Code, want)
			}
			c.call("DELETE", prefix+"transfers/"+c.id, nil)
		})
	}
}
func TestPlaintextRoutesNeverReturnFiles(t *testing.T) {
	marker := "private content"
	m := newTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(marker)) }), 0)
	guard := m.Guard(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("encrypted browser loader")) })
	for _, path := range []string{"/download/?path=ZmlsZQ==", "/raw/file", "/zip", "/zip-selected", "/file-content?path=ZmlsZQ==", "/search-file?path=ZmlsZQ==", "/custom-alias", "/screenshot/id", "/api/v1/screenshots/image?id=123"} {
		for _, accept := range []string{"*/*", "text/html"} {
			t.Run(path+accept, func(t *testing.T) {
				r := httptest.NewRequest("GET", path, nil)
				r.Header.Set("Accept", accept)
				w := httptest.NewRecorder()
				guard.ServeHTTP(w, r)
				if strings.Contains(w.Body.String(), marker) {
					t.Fatal("plaintext file escaped guard")
				}
				if accept == "*/*" && w.Code != 426 {
					t.Fatal(w.Code)
				}
			})
		}
	}
	for _, path := range []string{"/", "/api/v1/screenshots"} {
		w := httptest.NewRecorder()
		guard.ServeHTTP(w, httptest.NewRequest("POST", path, strings.NewReader(marker)))
		if w.Code != 426 {
			t.Fatal("plaintext upload accepted", w.Code)
		}
	}
}
func TestTransferExpiryAndOrigin(t *testing.T) {
	m := newTestManager(t, http.NotFoundHandler(), 0)
	c := newClient(t, m, "GET", "/download/?path=eA==", nil)
	name := m.items[c.id].file.Name()
	m.items[c.id].touched = time.Now().Add(-lifetime)
	m.expire(time.Now())
	if _, err := os.Stat(name); !os.IsNotExist(err) {
		t.Fatal("expired request file not removed")
	}
	if w := c.call("GET", prefix+"transfers/"+c.id+"/chunks?index=0", nil); w.Code != 410 {
		t.Fatal(w.Code)
	}
	r := httptest.NewRequest("GET", prefix+"key", nil)
	r.Header.Set("Origin", "https://untrusted.example")
	w := httptest.NewRecorder()
	m.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("cross-origin request accepted")
	}
}
func TestRequestTargetsAndHeaders(t *testing.T) {
	for _, target := range []string{"https://evil.example/raw/file", "//evil.example/raw/file", "/api/v1/crypto/key", "/clipboard/stream", "/raw/\\file"} {
		if validRequest(requestMetadata{Method: "GET", URL: target}) {
			t.Fatal("accepted target", target)
		}
	}
	for _, header := range []string{"Authorization", "Host", "Cookie", "X-Forwarded-For"} {
		if validRequest(requestMetadata{Method: "GET", URL: "/raw/file", Headers: map[string]string{header: "spoofed"}}) {
			t.Fatal("accepted header", header)
		}
	}
}

func TestEncryptedUploadsUseExistingPathAndReadonlyChecks(t *testing.T) {
	for _, tc := range []struct {
		name, filename string
		readonly       bool
		want           int
	}{
		{"nested file", "folder/private.txt", false, 201},
		{"traversal", "../escape.txt", false, 403},
		{"readonly", "private.txt", true, 403},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			hidden := false
			paths := map[string]string{}
			var mu sync.RWMutex
			fh := handlers.NewFileHandlers(dir, true, false, tc.readonly, 0, &hidden, &paths, &mu)
			m := newTestManager(t, fh.List(), 0)
			var data bytes.Buffer
			writer := multipart.NewWriter(&data)
			part, err := writer.CreateFormFile("file", tc.filename)
			if err != nil {
				t.Fatal(err)
			}
			_, _ = part.Write([]byte("private content"))
			_ = writer.Close()
			c := newClient(t, m, "POST", "/", map[string]string{"Content-Type": writer.FormDataContentType(), "X-Requested-With": "XMLHttpRequest"})
			c.upload(data.Bytes())
			meta, _ := c.complete()
			if meta.Status != tc.want {
				t.Fatalf("got %d, want %d", meta.Status, tc.want)
			}
			if tc.want == 201 {
				got, err := os.ReadFile(filepath.Join(dir, tc.filename))
				if err != nil || string(got) != "private content" {
					t.Fatalf("saved file: %q %v", got, err)
				}
			} else {
				entries, _ := os.ReadDir(dir)
				if len(entries) != 0 {
					t.Fatal("forbidden upload created a file")
				}
			}
		})
	}
}
func TestEncryptedEndpointRetainsBasicAuthentication(t *testing.T) {
	m := newTestManager(t, http.NotFoundHandler(), 0)
	handler := security.ApplyBasicAuth(m.ServeHTTP, "test", "password")
	for _, tc := range []struct {
		name, pass string
		want       int
	}{
		{"missing", "", 401}, {"wrong", "incorrect", 401}, {"valid", "password", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", prefix+"key", nil)
			if tc.pass != "" {
				r.SetBasicAuth("test", tc.pass)
			}
			w := httptest.NewRecorder()
			handler(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code)
			}
		})
	}
}
func TestConcurrentTransfersAndCleanup(t *testing.T) {
	m := newTestManager(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = io.Copy(w, r.Body) }), 0)
	for i := 0; i < 16; i++ {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			c := newClient(t, m, "POST", "/", nil)
			c.upload([]byte("private content"))
			_, body := c.complete()
			if string(body) != "private content" {
				t.Fatal("concurrent response corrupted")
			}
		})
	}
}

func TestWebCryptoProtocolVector(t *testing.T) {
	data, err := os.ReadFile("testdata/v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var v struct {
		ID, Kind, Key, Plaintext, Packet string
		Index                            int
	}
	if err = json.Unmarshal(data, &v); err != nil {
		t.Fatal(err)
	}
	key, err := hex.DecodeString(v.Key)
	if err != nil {
		t.Fatal(err)
	}
	a, err := newAEAD(key)
	if err != nil {
		t.Fatal(err)
	}
	packet, err := base64.StdEncoding.DecodeString(v.Packet)
	if err != nil {
		t.Fatal(err)
	}
	plain, err := openPacket(a, v.ID, v.Kind, v.Index, packet)
	if err != nil || string(plain) != v.Plaintext {
		t.Fatalf("Web Crypto vector: %q %v", plain, err)
	}
}
func TestInitiationCannotBeReplayed(t *testing.T) {
	m := newTestManager(t, http.NotFoundHandler(), 0)
	c := newClient(t, m, "GET", "/raw/file", nil)
	// Replay the exact original RSA envelope and authenticated metadata, both
	// while active and after cancellation. A fresh transfer needs fresh keys/id.
	for _, cancel := range []bool{false, true} {
		if cancel {
			c.call("DELETE", prefix+"transfers/"+c.id, nil)
		}
		w := c.call("POST", prefix+"transfers", c.initial)
		if w.Code != 409 {
			t.Fatal("accepted initiation replay", w.Code)
		}
	}
}
