package handlers

import (
	"log"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
)

// CustomPathHandler manages custom path creation
type CustomPathHandler struct {
	Dir              string
	Quite            bool
	CustomPaths      *map[string]string
	CustomPathsMutex *sync.RWMutex
}

// NewCustomPathHandler creates a new CustomPathHandler instance
func NewCustomPathHandler(dir string, quite bool, customPaths *map[string]string, customPathsMutex *sync.RWMutex) *CustomPathHandler {
	return &CustomPathHandler{
		Dir:              dir,
		Quite:            quite,
		CustomPaths:      customPaths,
		CustomPathsMutex: customPathsMutex,
	}
}

// Handle processes custom path creation requests
func (cph *CustomPathHandler) Handle() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !cph.Quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		originalPath := r.FormValue("originalPath")
		customPath := r.FormValue("customPath")

		// Validate custom path (no special characters)
		if !isValidCustomPath(customPath) {
			http.Error(w, "Invalid custom path", http.StatusBadRequest)
			return
		}

		// Check if custom path already exists
		cph.CustomPathsMutex.RLock()
		for _, existingPath := range *cph.CustomPaths {
			if existingPath == customPath {
				cph.CustomPathsMutex.RUnlock()
				http.Error(w, "Custom path already exists", http.StatusConflict)
				return
			}
		}
		cph.CustomPathsMutex.RUnlock()

		// Validate original path
		if originalPath == "" {
			http.Error(w, "Original path is required", http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(cph.Dir, originalPath)
		isSafe, err := security.IsSafePath(cph.Dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Invalid file path", http.StatusForbidden)
			return
		}

		// Store the custom path
		cph.CustomPathsMutex.Lock()
		(*cph.CustomPaths)[originalPath] = customPath
		cph.CustomPathsMutex.Unlock()

		if !cph.Quite {
			log.Printf("[%s] Custom path created: %s -> %s\n", time.Now().Format("2006-01-02 15:04:05"), customPath, originalPath)
		}

		// Redirect to the custom path
		http.Redirect(w, r, "/"+customPath, http.StatusSeeOther)
	}
}

// isValidCustomPath checks if a custom path contains only alphanumeric characters and hyphens
func isValidCustomPath(path string) bool {
	if path == "" {
		return false
	}
	for _, char := range path {
		if !((char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') || (char >= '0' && char <= '9') || char == '-' || char == '_') {
			return false
		}
	}
	return true
}
