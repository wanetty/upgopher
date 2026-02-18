package handlers

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net"
	"net/http"
	"regexp"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

var tabNameRegex = regexp.MustCompile(`^[a-zA-Z0-9 _-]{1,50}$`)

// ClipboardEntry holds the content and metadata for a single clipboard tab.
type ClipboardEntry struct {
	Content   string
	UpdatedAt time.Time
	TokenHash string // SHA-256 hex of the token; empty = no protection
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
	Quiet bool
	store *clipboardStore
}

// NewClipboardHandler creates a new ClipboardHandler with its own internal store.
func NewClipboardHandler(quiet bool, maxTabs int) *ClipboardHandler {
	return &ClipboardHandler{
		Quiet: quiet,
		store: newClipboardStore(maxTabs),
	}
}

// tabInfo is the JSON response item for /clipboard/tabs.
type tabInfo struct {
	Name      string    `json:"name"`
	Size      int       `json:"size"`
	UpdatedAt time.Time `json:"updatedAt"`
	Protected bool      `json:"protected"`
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
				Name:      name,
				Size:      len(entry.Content),
				UpdatedAt: entry.UpdatedAt,
				Protected: entry.Protected(),
			})
		}
		ch.store.mu.RUnlock()

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(list)
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

func setClipboardCORSHeaders(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Tab-Token, X-Tab-Token-Create")
}

func clipboardExtractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}
