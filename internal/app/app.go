package app

import "sync"

// Config holds the application configuration
type Config struct {
	Dir                string
	Port               int
	User               string
	Pass               string
	SSL                bool
	Cert               string
	Key                string
	Quite              bool
	DisableHiddenFiles bool
}

// App encapsulates the application state and configuration
type App struct {
	Config           *Config
	ShowHiddenFiles  bool
	CustomPaths      map[string]string
	CustomPathsMutex sync.RWMutex
	SharedClipboard  string
	ClipboardMutex   sync.Mutex
}

// NewApp creates a new App instance with the provided configuration
func NewApp(config *Config) *App {
	return &App{
		Config:          config,
		ShowHiddenFiles: false,
		CustomPaths:     make(map[string]string),
	}
}

// GetCustomPath retrieves a custom path by original path (thread-safe)
func (a *App) GetCustomPath(originalPath string) (string, bool) {
	a.CustomPathsMutex.RLock()
	defer a.CustomPathsMutex.RUnlock()
	customPath, exists := a.CustomPaths[originalPath]
	return customPath, exists
}

// SetCustomPath sets a custom path for an original path (thread-safe)
func (a *App) SetCustomPath(originalPath, customPath string) {
	a.CustomPathsMutex.Lock()
	defer a.CustomPathsMutex.Unlock()
	a.CustomPaths[originalPath] = customPath
}

// GetAllCustomPaths returns a copy of all custom paths (thread-safe)
func (a *App) GetAllCustomPaths() map[string]string {
	a.CustomPathsMutex.RLock()
	defer a.CustomPathsMutex.RUnlock()

	// Return a copy to avoid concurrent access issues
	copy := make(map[string]string, len(a.CustomPaths))
	for k, v := range a.CustomPaths {
		copy[k] = v
	}
	return copy
}

// CheckCustomPathExists checks if a custom path already exists (thread-safe)
func (a *App) CheckCustomPathExists(customPath string) bool {
	a.CustomPathsMutex.RLock()
	defer a.CustomPathsMutex.RUnlock()

	for _, existingPath := range a.CustomPaths {
		if existingPath == customPath {
			return true
		}
	}
	return false
}

// GetClipboard retrieves the shared clipboard content (thread-safe)
func (a *App) GetClipboard() string {
	a.ClipboardMutex.Lock()
	defer a.ClipboardMutex.Unlock()
	return a.SharedClipboard
}

// SetClipboard sets the shared clipboard content (thread-safe)
func (a *App) SetClipboard(content string) {
	a.ClipboardMutex.Lock()
	defer a.ClipboardMutex.Unlock()
	a.SharedClipboard = content
}

// ToggleHiddenFiles toggles the ShowHiddenFiles flag
func (a *App) ToggleHiddenFiles() {
	a.ShowHiddenFiles = !a.ShowHiddenFiles
}

// GetShowHiddenFiles returns the current value of ShowHiddenFiles
func (a *App) GetShowHiddenFiles() bool {
	return a.ShowHiddenFiles
}
