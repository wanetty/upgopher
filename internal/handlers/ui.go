package handlers

import (
	"embed"
	"log"
	"net/http"
	"time"
)

// UIHandlers manages UI-related HTTP handlers (favicon, logo, settings toggle)
type UIHandlers struct {
	Quite              bool
	DisableHiddenFiles bool
	ShowHiddenFiles    *bool
	FaviconFS          *embed.FS
	LogoFS             *embed.FS
}

// NewUIHandlers creates a new UIHandlers instance
func NewUIHandlers(quite bool, disableHiddenFiles bool, showHiddenFiles *bool, faviconFS *embed.FS, logoFS *embed.FS) *UIHandlers {
	return &UIHandlers{
		Quite:              quite,
		DisableHiddenFiles: disableHiddenFiles,
		ShowHiddenFiles:    showHiddenFiles,
		FaviconFS:          faviconFS,
		LogoFS:             logoFS,
	}
}

// Favicon serves the favicon
func (ui *UIHandlers) Favicon() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		faviconData, err := ui.FaviconFS.ReadFile("static/favicon.ico")
		if err != nil {
			http.Error(w, "Favicon not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/x-icon")
		w.Write(faviconData)
	}
}

// Logo serves the logo image
func (ui *UIHandlers) Logo() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		logoData, err := ui.LogoFS.ReadFile("static/logopher.webp")
		if err != nil {
			http.Error(w, "Logo not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		w.Write(logoData)
	}
}

// ToggleHiddenFiles handles GET/POST for hidden files visibility
func (ui *UIHandlers) ToggleHiddenFiles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Handle GET request - return current hidden files status
		if r.Method == http.MethodGet {
			if !ui.Quite {
				log.Printf("[%s] Getting hidden files setting: %t\n", time.Now().Format("2006-01-02 15:04:05"), *ui.ShowHiddenFiles)
			}
			if *ui.ShowHiddenFiles {
				w.Write([]byte("true"))
				return
			} else {
				w.Write([]byte("false"))
			}
		} else if r.Method == http.MethodPost {
			// Handle POST request - toggle hidden files setting
			if !ui.Quite {
				log.Printf("[%s] Toggling hidden files setting\n", time.Now().Format("2006-01-02 15:04:05"))
			}
			if ui.DisableHiddenFiles {
				http.Error(w, "You can't change this setting", http.StatusForbidden)
				return
			} else {
				*ui.ShowHiddenFiles = !*ui.ShowHiddenFiles
				return
			}
		}
	}
}
