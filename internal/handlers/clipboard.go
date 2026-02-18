package handlers

import (
	"io"
	"log"
	"net"
	"net/http"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

// ClipboardHandler manages shared clipboard HTTP endpoint
type ClipboardHandler struct {
	Quiet           bool
	SharedClipboard *string
	ClipboardMutex  interface {
		Lock()
		Unlock()
	}
}

// NewClipboardHandler creates a new ClipboardHandler instance
func NewClipboardHandler(quiet bool, sharedClipboard *string, clipboardMutex interface {
	Lock()
	Unlock()
}) *ClipboardHandler {
	return &ClipboardHandler{
		Quiet:           quiet,
		SharedClipboard: sharedClipboard,
		ClipboardMutex:  clipboardMutex,
	}
}

// Handle processes clipboard GET/POST requests with rate limiting
func (ch *ClipboardHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !ch.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		// Set CORS headers to allow requests from any origin
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		// Handle preflight OPTIONS request
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		if r.Method == http.MethodGet {
			// Return current clipboard content
			ch.ClipboardMutex.Lock()
			clipboard := *ch.SharedClipboard
			ch.ClipboardMutex.Unlock()

			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte(clipboard))
			if !ch.Quiet {
				log.Printf("[%s] Clipboard content returned (%d chars)\n", time.Now().Format("2006-01-02 15:04:05"), len(clipboard))
			}
		} else if r.Method == http.MethodPost {
			// Extract IP without port for accurate rate limiting
			clientIP, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				clientIP = r.RemoteAddr
			}
			if !security.CheckRateLimit(clientIP) {
				http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
				if !ch.Quiet {
					log.Printf("[%s] Rate limit exceeded for IP: %s\n", time.Now().Format("2006-01-02 15:04:05"), clientIP)
				}
				return
			}

			// Limit body size to 1MB to prevent memory exhaustion
			r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			defer r.Body.Close()

			// Update clipboard with received data
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, "Error reading data", http.StatusBadRequest)
				log.Printf("[%s] Error reading clipboard data: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
				return
			}

			ch.ClipboardMutex.Lock()
			*ch.SharedClipboard = string(body)
			ch.ClipboardMutex.Unlock()

			w.WriteHeader(http.StatusOK)
			if !ch.Quiet {
				log.Printf("[%s] Clipboard updated (%d chars)\n", time.Now().Format("2006-01-02 15:04:05"), len(body))
			}
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
