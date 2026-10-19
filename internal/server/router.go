package server

import (
	"embed"
	"log"
	"net/http"
	"sync"

	"github.com/wanetty/upgopher/internal/handlers"
	"github.com/wanetty/upgopher/internal/security"
	"github.com/wanetty/upgopher/internal/statics"
	"github.com/wanetty/upgopher/internal/transfers"
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
	maxUploadSize int64,
	showHiddenFiles *bool,
	customPaths *map[string]string,
	customPathsMutex *sync.RWMutex,
	faviconFS *embed.FS,
	logoFS *embed.FS,
	encrypted ...bool,
) (func(), error) {
	mux := http.NewServeMux()
	registerRoute := func(pattern string, h http.Handler) { mux.Handle(pattern, h) }
	fileHandlers := handlers.NewFileHandlers(dir, quiet, disableHiddenFiles, readOnly, maxUploadSize, showHiddenFiles, customPaths, customPathsMutex)
	fileHandlers.EncryptTransfers = len(encrypted) > 0 && encrypted[0]
	clipboardHandler := handlers.NewClipboardHandler(quiet, maxTabs)
	customPathHandler := handlers.NewCustomPathHandler(dir, quiet, customPaths, customPathsMutex)
	uiHandlers := handlers.NewUIHandlers(quiet, disableHiddenFiles, readOnly, showHiddenFiles, faviconFS, logoFS)

	registerRoute("/", fileHandlers.List())
	registerRoute("/download/", http.StripPrefix("/download/", fileHandlers.Download()))
	registerRoute("/delete/", http.StripPrefix("/delete/", fileHandlers.Delete()))
	registerRoute("/raw/", http.StripPrefix("/raw/", fileHandlers.Raw()))
	registerRoute("/zip", fileHandlers.Zip())
	registerRoute("/zip-selected", fileHandlers.ZipSelected())
	registerRoute("/file-content", fileHandlers.FileContent())
	registerRoute("/search-file", fileHandlers.Search())
	registerRoute("/api/v1/breadcrumbs", fileHandlers.Breadcrumbs())
	registerRoute("/api/v1/tree", fileHandlers.Tree())
	registerRoute("/clipboard/tabs", clipboardHandler.ListTabs())
	registerRoute("/clipboard/stream", clipboardHandler.ClipboardStream())
	registerRoute("/api/v1/screenshots/image", clipboardHandler.ScreenshotImage())
	registerRoute("/api/v1/screenshots", clipboardHandler.Screenshots())
	registerRoute("/screenshot/", http.StripPrefix("/screenshot/", clipboardHandler.ServeScreenshotDirect()))
	registerRoute("/clipboard", clipboardHandler.Handle())
	registerRoute("/mkdir", fileHandlers.Mkdir())
	registerRoute("/custom-path", customPathHandler.Handle())
	registerRoute("/showhiddenfiles", uiHandlers.ToggleHiddenFiles())
	registerRoute("/favicon.ico", uiHandlers.Favicon())
	registerRoute("/static/logopher.webp", uiHandlers.Logo())
	cleanup := func() {}
	var root http.Handler = mux
	if fileHandlers.EncryptTransfers {
		manager, err := transfers.New(mux, maxUploadSize)
		if err != nil {
			return cleanup, err
		}
		if !quiet {
			log.Printf("Transfer encryption enabled (passive observers only). Server fingerprint: %s", manager.Fingerprint())
		}
		root = manager.Guard(statics.TransferPage)
		cleanup = manager.Close
	}
	registerRouteOnDefault("/", root, user, pass)
	return cleanup, nil
}

// registerRouteOnDefault wraps handler with authentication if credentials are provided
func registerRouteOnDefault(pattern string, handler http.Handler, user string, pass string) {
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
