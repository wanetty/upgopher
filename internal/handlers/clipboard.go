package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

var tabNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,50}$`)

const maxClipboardImageSize = 16 << 20

// ClipboardEntry holds the content and metadata for a single clipboard tab.
type ClipboardEntry struct {
	Content          string
	UpdatedAt        time.Time
	ImageData        []byte
	ImageContentType string
	ImageUpdatedAt   time.Time
	TokenHash        string // SHA-256 hex of the token; empty = no protection
}

// Protected reports whether this tab requires a token to access.
func (e *ClipboardEntry) Protected() bool {
	return e.TokenHash != ""
}

// clipboardStore holds all clipboard tabs with thread-safe access.
type clipboardStore struct {
	tabs    map[string]*ClipboardEntry
	mu      sync.RWMutex
	maxTabs int
}

func newClipboardStore(maxTabs int) *clipboardStore {
	s := &clipboardStore{
		tabs:    make(map[string]*ClipboardEntry),
		maxTabs: maxTabs,
	}
	s.tabs["default"] = &ClipboardEntry{UpdatedAt: time.Now()}
	return s
}

// ── SSE Broker ────────────────────────────────────────────────────────────────

// clipboardBroker manages Server-Sent Event subscribers per clipboard tab.
// Each subscriber receives a buffered channel; Broadcast drops the notification
// if the subscriber is too slow (non-blocking send), so it can never block the
// writer goroutine.
type clipboardBroker struct {
	mu          sync.Mutex
	subscribers map[string][]chan struct{}
}

func newClipboardBroker() *clipboardBroker {
	return &clipboardBroker{
		subscribers: make(map[string][]chan struct{}),
	}
}

// Subscribe registers a new subscriber for tabName and returns
// a receive-only channel plus an unsubscribe function.
func (b *clipboardBroker) Subscribe(tabName string) (<-chan struct{}, func()) {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	b.subscribers[tabName] = append(b.subscribers[tabName], ch)
	b.mu.Unlock()
	return ch, func() { b.unsubscribe(tabName, ch) }
}

func (b *clipboardBroker) unsubscribe(tabName string, ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	subs := b.subscribers[tabName]
	for i, s := range subs {
		if s == ch {
			b.subscribers[tabName] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	if len(b.subscribers[tabName]) == 0 {
		delete(b.subscribers, tabName)
	}
}

// Broadcast notifies all subscribers of tabName that its content changed.
func (b *clipboardBroker) Broadcast(tabName string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, ch := range b.subscribers[tabName] {
		select {
		case ch <- struct{}{}:
		default: // subscriber is slow; skip — it will catch up on next event
		}
	}
}

// generateToken returns a cryptographically random token (64 hex chars) and its
// SHA-256 hash (also hex). The plaintext is returned once to the caller; only
// the hash is stored.
func generateToken() (plain, hash string, err error) {
	b := make([]byte, 32)
	if _, err = rand.Read(b); err != nil {
		return
	}
	plain = hex.EncodeToString(b)
	sum := sha256.Sum256([]byte(plain))
	hash = hex.EncodeToString(sum[:])
	return
}

// tokenHash computes the SHA-256 hex hash of a plaintext token value.
func tokenHash(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}

// checkTabToken returns true when access to entry should be granted.
// If the tab has no token, access is always granted.
// Otherwise the request must carry a matching X-Tab-Token header (constant-time compare).
func checkTabToken(entry *ClipboardEntry, r *http.Request) bool {
	if !entry.Protected() {
		return true
	}
	provided := r.Header.Get("X-Tab-Token")
	if provided == "" {
		return false
	}
	provided = tokenHash(provided)
	return subtle.ConstantTimeCompare([]byte(provided), []byte(entry.TokenHash)) == 1
}

// ClipboardHandler manages shared clipboard HTTP endpoints.
type ClipboardHandler struct {
	Quiet  bool
	store  *clipboardStore
	broker *clipboardBroker
}

// NewClipboardHandler creates a new ClipboardHandler with its own internal store.
func NewClipboardHandler(quiet bool, maxTabs int) *ClipboardHandler {
	return &ClipboardHandler{
		Quiet:  quiet,
		store:  newClipboardStore(maxTabs),
		broker: newClipboardBroker(),
	}
}

// tabInfo is the JSON response item for /clipboard/tabs.
type tabInfo struct {
	Name             string    `json:"name"`
	Size             int       `json:"size"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Protected        bool      `json:"protected"`
	ImageSize        int       `json:"imageSize"`
	ImageContentType string    `json:"imageContentType,omitempty"`
	ImageUpdatedAt   time.Time `json:"imageUpdatedAt,omitempty"`
}

// ListTabs handles GET /clipboard/tabs — returns JSON array of tab metadata.
func (ch *ClipboardHandler) ListTabs() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ch.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}
		setClipboardCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ch.store.mu.RLock()
		list := make([]tabInfo, 0, len(ch.store.tabs))
		for name, entry := range ch.store.tabs {
			list = append(list, tabInfo{
				Name:             name,
				Size:             len(entry.Content),
				UpdatedAt:        entry.UpdatedAt,
				Protected:        entry.Protected(),
				ImageSize:        len(entry.ImageData),
				ImageContentType: entry.ImageContentType,
				ImageUpdatedAt:   entry.ImageUpdatedAt,
			})
		}
		ch.store.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
	}
}

// ClipboardImage handles GET/POST/DELETE on /clipboard/image with optional ?tab=<name>.
func (ch *ClipboardHandler) ClipboardImage() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ch.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}
		setClipboardCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		tabName := r.URL.Query().Get("tab")
		if tabName == "" {
			tabName = "default"
		}

		switch r.Method {
		case http.MethodGet:
			ch.handleImageGet(w, r, tabName)
		case http.MethodPost:
			ch.handleImagePost(w, r, tabName)
		case http.MethodDelete:
			ch.handleImageDelete(w, r, tabName)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// Handle processes GET/POST/DELETE requests on /clipboard with optional ?tab=<name>.
func (ch *ClipboardHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ch.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}
		setClipboardCORSHeaders(w)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		tabName := r.URL.Query().Get("tab")
		if tabName == "" {
			tabName = "default"
		}

		switch r.Method {
		case http.MethodGet:
			ch.handleGet(w, r, tabName)
		case http.MethodPost:
			ch.handlePost(w, r, tabName)
		case http.MethodDelete:
			ch.handleDelete(w, r, tabName)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func (ch *ClipboardHandler) handleGet(w http.ResponseWriter, r *http.Request, tabName string) {
	// Copy the fields we need while holding the lock to avoid data races.
	ch.store.mu.RLock()
	entry, ok := ch.store.tabs[tabName]
	var content, hash string
	if ok {
		content = entry.Content
		hash = entry.TokenHash
	}
	ch.store.mu.RUnlock()

	if !ok {
		http.Error(w, "Tab not found", http.StatusNotFound)
		return
	}

	snap := &ClipboardEntry{TokenHash: hash}
	if !checkTabToken(snap, r) {
		w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(content))
	if !ch.Quiet {
		log.Printf("[%s] Clipboard tab %q returned (%d chars)\n", time.Now().Format("2006-01-02 15:04:05"), tabName, len(content))
	}
}

func (ch *ClipboardHandler) handlePost(w http.ResponseWriter, r *http.Request, tabName string) {
	if !tabNameRegex.MatchString(tabName) {
		http.Error(w, "Invalid tab name", http.StatusBadRequest)
		return
	}

	clientIP := clipboardExtractIP(r)
	if !security.CheckRateLimit(clientIP) {
		http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
		if !ch.Quiet {
			log.Printf("[%s] Rate limit exceeded for IP: %s\n", time.Now().Format("2006-01-02 15:04:05"), clientIP)
		}
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading data", http.StatusBadRequest)
		return
	}

	wantsToken := r.Header.Get("X-Tab-Token-Create") == "1"
	customToken := r.Header.Get("X-Tab-Token-Value") // optional: user-defined password

	ch.store.mu.Lock()
	existing, exists := ch.store.tabs[tabName]

	// Creating a new tab
	if !exists {
		if len(ch.store.tabs) >= ch.store.maxTabs {
			ch.store.mu.Unlock()
			http.Error(w, "Maximum number of tabs reached", http.StatusForbidden)
			return
		}
		entry := &ClipboardEntry{Content: string(body), UpdatedAt: time.Now()}
		if wantsToken {
			if customToken != "" {
				// User-defined token: validate minimum length then hash and store.
				if len(customToken) < 6 {
					ch.store.mu.Unlock()
					http.Error(w, "Custom token must be at least 6 characters", http.StatusBadRequest)
					return
				}
				sum := sha256.Sum256([]byte(customToken))
				entry.TokenHash = hex.EncodeToString(sum[:])
				ch.store.tabs[tabName] = entry
				ch.store.mu.Unlock()
				// No X-Generated-Token header — user already knows their own token.
				w.WriteHeader(http.StatusCreated)
				if !ch.Quiet {
					log.Printf("[%s] Clipboard tab %q created (protected, custom token)\n", time.Now().Format("2006-01-02 15:04:05"), tabName)
				}
				return
			}
			// Auto-generated token (existing behaviour).
			plain, hash, genErr := generateToken()
			if genErr != nil {
				ch.store.mu.Unlock()
				http.Error(w, "Failed to generate token", http.StatusInternalServerError)
				return
			}
			entry.TokenHash = hash
			ch.store.tabs[tabName] = entry
			ch.store.mu.Unlock()
			w.Header().Set("X-Generated-Token", plain)
			w.WriteHeader(http.StatusCreated)
			if !ch.Quiet {
				log.Printf("[%s] Clipboard tab %q created (protected)\n", time.Now().Format("2006-01-02 15:04:05"), tabName)
			}
			return
		}
		ch.store.tabs[tabName] = entry
		ch.store.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		if !ch.Quiet {
			log.Printf("[%s] Clipboard tab %q created\n", time.Now().Format("2006-01-02 15:04:05"), tabName)
		}
		return
	}

	// Updating an existing tab — check token before writing
	if !checkTabToken(existing, r) {
		ch.store.mu.Unlock()
		w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	existing.Content = string(body)
	existing.UpdatedAt = time.Now()
	ch.store.mu.Unlock()

	ch.broker.Broadcast(tabName)

	w.WriteHeader(http.StatusOK)
	if !ch.Quiet {
		log.Printf("[%s] Clipboard tab %q updated (%d chars)\n", time.Now().Format("2006-01-02 15:04:05"), tabName, len(body))
	}
}

func (ch *ClipboardHandler) handleDelete(w http.ResponseWriter, r *http.Request, tabName string) {
	if tabName == "default" {
		http.Error(w, "Cannot delete default tab", http.StatusForbidden)
		return
	}

	clientIP := clipboardExtractIP(r)
	if !security.CheckRateLimit(clientIP) {
		http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
		return
	}

	ch.store.mu.Lock()
	entry, ok := ch.store.tabs[tabName]
	if ok {
		if !checkTabToken(entry, r) {
			ch.store.mu.Unlock()
			w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		delete(ch.store.tabs, tabName)
	}
	ch.store.mu.Unlock()

	if !ok {
		http.Error(w, "Tab not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusOK)
	if !ch.Quiet {
		log.Printf("[%s] Clipboard tab %q deleted\n", time.Now().Format("2006-01-02 15:04:05"), tabName)
	}
}

func (ch *ClipboardHandler) handleImageGet(w http.ResponseWriter, r *http.Request, tabName string) {
	ch.store.mu.RLock()
	entry, ok := ch.store.tabs[tabName]
	var imageData []byte
	var contentType, hash string
	if ok {
		imageData = append([]byte(nil), entry.ImageData...)
		contentType = entry.ImageContentType
		hash = entry.TokenHash
	}
	ch.store.mu.RUnlock()

	if !ok {
		http.Error(w, "Tab not found", http.StatusNotFound)
		return
	}

	snap := &ClipboardEntry{TokenHash: hash}
	if !checkTabToken(snap, r) {
		w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	if len(imageData) == 0 {
		http.Error(w, "Image not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store")
	w.Write(imageData)
}

func (ch *ClipboardHandler) handleImagePost(w http.ResponseWriter, r *http.Request, tabName string) {
	if !tabNameRegex.MatchString(tabName) {
		http.Error(w, "Invalid tab name", http.StatusBadRequest)
		return
	}

	clientIP := clipboardExtractIP(r)
	if !security.CheckRateLimit(clientIP) {
		http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxClipboardImageSize)
	defer r.Body.Close()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Error reading data", http.StatusBadRequest)
		return
	}
	contentType := normalizeClipboardImageContentType(r.Header.Get("Content-Type"), body)
	if len(body) == 0 || contentType == "" {
		http.Error(w, "Unsupported image type", http.StatusUnsupportedMediaType)
		return
	}

	wantsToken := r.Header.Get("X-Tab-Token-Create") == "1"
	customToken := r.Header.Get("X-Tab-Token-Value")

	ch.store.mu.Lock()
	existing, exists := ch.store.tabs[tabName]
	if !exists {
		if len(ch.store.tabs) >= ch.store.maxTabs {
			ch.store.mu.Unlock()
			http.Error(w, "Maximum number of tabs reached", http.StatusForbidden)
			return
		}
		now := time.Now()
		entry := &ClipboardEntry{
			UpdatedAt:        now,
			ImageData:        append([]byte(nil), body...),
			ImageContentType: contentType,
			ImageUpdatedAt:   now,
		}
		if wantsToken {
			if customToken != "" {
				if len(customToken) < 6 {
					ch.store.mu.Unlock()
					http.Error(w, "Custom token must be at least 6 characters", http.StatusBadRequest)
					return
				}
				entry.TokenHash = tokenHash(customToken)
			} else {
				plain, hash, genErr := generateToken()
				if genErr != nil {
					ch.store.mu.Unlock()
					http.Error(w, "Failed to generate token", http.StatusInternalServerError)
					return
				}
				entry.TokenHash = hash
				w.Header().Set("X-Generated-Token", plain)
			}
		}
		ch.store.tabs[tabName] = entry
		ch.store.mu.Unlock()
		w.WriteHeader(http.StatusCreated)
		ch.broker.Broadcast(tabName)
		return
	}

	if !checkTabToken(existing, r) {
		ch.store.mu.Unlock()
		w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}
	existing.ImageData = append(existing.ImageData[:0], body...)
	existing.ImageContentType = contentType
	existing.ImageUpdatedAt = time.Now()
	ch.store.mu.Unlock()

	ch.broker.Broadcast(tabName)
	w.WriteHeader(http.StatusOK)
}

func (ch *ClipboardHandler) handleImageDelete(w http.ResponseWriter, r *http.Request, tabName string) {
	clientIP := clipboardExtractIP(r)
	if !security.CheckRateLimit(clientIP) {
		http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
		return
	}

	ch.store.mu.Lock()
	entry, ok := ch.store.tabs[tabName]
	if ok {
		if !checkTabToken(entry, r) {
			ch.store.mu.Unlock()
			w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		entry.ImageData = nil
		entry.ImageContentType = ""
		entry.ImageUpdatedAt = time.Time{}
	}
	ch.store.mu.Unlock()

	if !ok {
		http.Error(w, "Tab not found", http.StatusNotFound)
		return
	}
	ch.broker.Broadcast(tabName)
	w.WriteHeader(http.StatusOK)
}

// ClipboardStream handles GET /clipboard/stream?tab=<name> for Server-Sent Events.
// Each connected client receives a "change" event whenever the tab content is updated.
func (ch *ClipboardHandler) ClipboardStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ch.Quiet {
			log.Printf("[%s] [SSE] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.RemoteAddr)
		}

		// Only GET is allowed for SSE.
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		tabName := r.URL.Query().Get("tab")
		if tabName == "" {
			tabName = "default"
		}

		// Verify the tab exists.
		ch.store.mu.RLock()
		entry, ok := ch.store.tabs[tabName]
		var tokenHash string
		if ok {
			tokenHash = entry.TokenHash
		}
		ch.store.mu.RUnlock()
		if !ok {
			http.Error(w, "Tab not found", http.StatusNotFound)
			return
		}

		// Check token on protected tabs.
		// EventSource cannot set custom request headers, so for SSE connections
		// the token is also accepted via the "X-Tab-Token" query parameter.
		// Regular REST handlers continue to use the header only (via checkTabToken).
		if tokenHash != "" {
			provided := r.Header.Get("X-Tab-Token")
			if provided == "" {
				provided = r.URL.Query().Get("X-Tab-Token")
			}
			hashed := ""
			if provided != "" {
				sum := sha256.Sum256([]byte(provided))
				hashed = hex.EncodeToString(sum[:])
			}
			if subtle.ConstantTimeCompare([]byte(hashed), []byte(tokenHash)) != 1 {
				w.Header().Set("WWW-Authenticate", `TabToken realm="Tab "`+tabName+`"`)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		// Ensure the ResponseWriter supports flushing.
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Set SSE headers.
		// NOTE: Do NOT set "Connection: keep-alive" — it is forbidden in HTTP/2
		// and causes ERR_HTTP2_PROTOCOL_ERROR in Chrome even on HTTP/1.1 connections.
		setClipboardCORSHeaders(w)
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no") // disable nginx buffering
		w.WriteHeader(http.StatusOK)

		// Subscribe to tab change notifications.
		notify, unsubscribe := ch.broker.Subscribe(tabName)
		defer unsubscribe()

		// Send an initial heartbeat so the client knows the connection is open.
		fmt.Fprintf(w, ": connected\n\n")
		flusher.Flush()

		// deadlineResetter lets us extend the write deadline on each heartbeat
		// so the server-level WriteTimeout (60 s) doesn't kill long-lived SSE connections.
		// This interface is implemented by Go's internal http.response type since Go 1.8.
		type deadlineWriter interface {
			SetWriteDeadline(t time.Time) error
		}
		dw, canReset := w.(deadlineWriter)

		// Heartbeat ticker to keep the connection alive through proxies and to
		// reset the write deadline before the server timeout fires.
		ticker := time.NewTicker(25 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				// Client disconnected — unsubscribe is called by defer.
				return
			case <-notify:
				// Tab content changed: send a "change" event.
				if canReset {
					dw.SetWriteDeadline(time.Now().Add(55 * time.Second))
				}
				fmt.Fprintf(w, "event: change\ndata: %s\n\n", tabName)
				flusher.Flush()
			case <-ticker.C:
				// Heartbeat comment to prevent proxy timeouts.
				// Also push the write deadline forward so the 60 s WriteTimeout
				// doesn't kill the connection between events.
				if canReset {
					dw.SetWriteDeadline(time.Now().Add(55 * time.Second))
				}
				fmt.Fprintf(w, ": heartbeat\n\n")
				flusher.Flush()
			}
		}
	}
}

func setClipboardCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Tab-Token, X-Tab-Token-Create, X-Tab-Token-Value")
}

func normalizeClipboardImageContentType(headerValue string, data []byte) string {
	headerValue = strings.ToLower(strings.TrimSpace(strings.Split(headerValue, ";")[0]))
	detected := strings.ToLower(http.DetectContentType(data))

	for _, candidate := range []string{detected, headerValue} {
		if isAllowedClipboardImageContentType(candidate, data) {
			return candidate
		}
	}
	return ""
}

func isAllowedClipboardImageContentType(contentType string, data []byte) bool {
	switch contentType {
	case "image/png":
		return len(data) >= 8 &&
			data[0] == 0x89 && data[1] == 'P' && data[2] == 'N' && data[3] == 'G' &&
			data[4] == '\r' && data[5] == '\n' && data[6] == 0x1a && data[7] == '\n'
	case "image/jpeg":
		return len(data) >= 3 && data[0] == 0xff && data[1] == 0xd8 && data[2] == 0xff
	case "image/gif":
		return len(data) >= 6 && (string(data[0:6]) == "GIF87a" || string(data[0:6]) == "GIF89a")
	case "image/webp":
		return len(data) >= 12 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WEBP"
	default:
		return false
	}
}

func clipboardExtractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
