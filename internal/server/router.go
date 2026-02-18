package server

import (
	"embed"
	"net/http"
	"sync"

	"github.com/wanetty/upgopher/internal/handlers"
	"github.com/wanetty/upgopher/internal/security"
)

// SetupRoutes initializes all HTTP routes with optional authentication
func SetupRoutes(
	dir string,
	user string,
	pass string,
	quiet bool,
	disableHiddenFiles bool,
	readOnly bool,
	maxTabs int,
	showHiddenFiles *bool,
	customPaths *map[string]string,
	customPathsMutex *sync.RWMutex,
	faviconFS *embed.FS,
	logoFS *embed.FS,
) {
	fileHandlers := handlers.NewFileHandlers(dir, quiet, disableHiddenFiles, readOnly, showHiddenFiles, customPaths, customPathsMutex)
	clipboardHandler := handlers.NewClipboardHandler(quiet, maxTabs)
	customPathHandler := handlers.NewCustomPathHandler(dir, quiet, customPaths, customPathsMutex)
	uiHandlers := handlers.NewUIHandlers(quiet, disableHiddenFiles, readOnly, showHiddenFiles, faviconFS, logoFS)

	registerRoute("/", fileHandlers.List(), user, pass)
	registerRoute("/download/", http.StripPrefix("/download/", fileHandlers.Download()), user, pass)
	registerRoute("/delete/", http.StripPrefix("/delete/", fileHandlers.Delete()), user, pass)
	registerRoute("/raw/", http.StripPrefix("/raw/", fileHandlers.Raw()), user, pass)
	registerRoute("/zip", fileHandlers.Zip(), user, pass)
	registerRoute("/search-file", fileHandlers.Search(), user, pass)
	registerRoute("/clipboard/tabs", clipboardHandler.ListTabs(), user, pass)
	registerRoute("/clipboard", clipboardHandler.Handle(), user, pass)
	registerRoute("/custom-path", customPathHandler.Handle(), user, pass)
	registerRoute("/showhiddenfiles", uiHandlers.ToggleHiddenFiles(), user, pass)
	registerRoute("/favicon.ico", uiHandlers.Favicon(), user, pass)
	registerRoute("/static/logopher.webp", uiHandlers.Logo(), user, pass)
}

// registerRoute wraps handler with authentication if credentials are provided
func registerRoute(pattern string, handler http.Handler, user string, pass string) {
	if user != "" && pass != "" {
		http.Handle(pattern, security.ApplyBasicAuth(convertToHandlerFunc(handler), user, pass))
	} else {
		http.Handle(pattern, handler)
	}
}

// convertToHandlerFunc converts http.Handler to http.HandlerFunc
func convertToHandlerFunc(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h.ServeHTTP(w, r)
	}
}
