package handlers

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
	"github.com/wanetty/upgopher/internal/statics"
	"github.com/wanetty/upgopher/internal/templates"
	"github.com/wanetty/upgopher/internal/utils"
)

// FileHandlers manages file-related HTTP handlers
type FileHandlers struct {
	Dir                string
	Quite              bool
	DisableHiddenFiles bool
	ReadOnly           bool
	ShowHiddenFiles    *bool
	CustomPaths        *map[string]string
	CustomPathsMutex   *sync.RWMutex
}

// NewFileHandlers creates a new FileHandlers instance
func NewFileHandlers(dir string, quite bool, disableHiddenFiles bool, readOnly bool, showHiddenFiles *bool, customPaths *map[string]string, customPathsMutex *sync.RWMutex) *FileHandlers {
	return &FileHandlers{
		Dir:                dir,
		Quite:              quite,
		DisableHiddenFiles: disableHiddenFiles,
		ReadOnly:           readOnly,
		ShowHiddenFiles:    showHiddenFiles,
		CustomPaths:        customPaths,
		CustomPathsMutex:   customPathsMutex,
	}
}

// List handles the root file listing endpoint
func (fh *FileHandlers) List() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !fh.Quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		// Check if it's a custom path
		requestPath := strings.TrimPrefix(r.URL.Path, "/")
		fh.CustomPathsMutex.RLock()
		for originalPath, customPath := range *fh.CustomPaths {
			if requestPath == customPath {
				fh.CustomPathsMutex.RUnlock()

				// Serve the file directly for download
				fullFilePath := filepath.Join(fh.Dir, originalPath)

				// Verify path safety
				isSafe, err := security.IsSafePath(fh.Dir, fullFilePath)
				if err != nil || !isSafe {
					http.Error(w, "Bad path", http.StatusForbidden)
					return
				}

				// Check if file exists
				if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
					http.Error(w, "File not found", http.StatusNotFound)
					return
				}

				// Serve the file with download header
				_, filename := filepath.Split(fullFilePath)
				w.Header().Set("Content-Disposition", "attachment; filename="+filename)
				http.ServeFile(w, r, fullFilePath)
				return
			}
		}
		fh.CustomPathsMutex.RUnlock()

		currentPath := r.URL.Query().Get("path")
		var newdir string

		if currentPath == "" {
			newdir = fh.Dir
		} else {
			decodedFilePath, err := base64.StdEncoding.DecodeString(currentPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			newdir = filepath.Join(fh.Dir, string(decodedFilePath))

			isSafe, err := security.IsSafePath(fh.Dir, newdir)
			if err != nil || !isSafe {
				http.Error(w, "Bad path", http.StatusForbidden)
				return
			}
		}

		fileInfo, err := os.Stat(newdir)
		if os.IsNotExist(err) || !fileInfo.IsDir() {
			http.Error(w, "The path does not exist", http.StatusInternalServerError)
			return
		}

		if r.Method == "GET" {
			fh.handleGetRequest(w, r, newdir, currentPath)
		} else if r.Method == "POST" {
			fh.handlePostRequest(w, r, newdir, currentPath)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// Raw serves files without download header
func (fh *FileHandlers) Raw() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		return_code := "200"
		path := strings.TrimPrefix(r.URL.Path, "/raw/")
		fullPath := filepath.Join(fh.Dir, path)

		isSafe, err := security.IsSafePath(fh.Dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			return_code = "403"
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) || fileInfo.IsDir() {
			http.Error(w, "File not found", http.StatusNotFound)
			return_code = "404"
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}
		if !fh.Quite {
			log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
		}
		http.ServeFile(w, r, fullPath)
	}
}

// Download serves files with attachment header
func (fh *FileHandlers) Download() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !fh.Quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		encodedFilePath := r.URL.Query().Get("path")
		decodedFilePath, err := base64.StdEncoding.DecodeString(encodedFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fullFilePath := filepath.Join(fh.Dir, string(decodedFilePath))
		isSafe, err := security.IsSafePath(fh.Dir, fullFilePath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "404", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		_, filename := filepath.Split(fullFilePath)
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		http.ServeFile(w, r, fullFilePath)
	}
}

// Delete removes a file
func (fh *FileHandlers) Delete() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Check if readonly mode is enabled
		if fh.ReadOnly {
			http.Error(w, "Delete operation is disabled in readonly mode", http.StatusForbidden)
			if !fh.Quite {
				log.Printf("[%s] Delete attempt blocked (readonly mode): %s\n", time.Now().Format("2006-01-02 15:04:05"), r.URL.String())
			}
			return
		}

		if !fh.Quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		encodedFilePath := r.URL.Query().Get("path")
		decodedFilePath, err := base64.StdEncoding.DecodeString(encodedFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("[%s] Error decoding path: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}

		fullFilePath := filepath.Join(fh.Dir, string(decodedFilePath))
		isSafe, err := security.IsSafePath(fh.Dir, fullFilePath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullFilePath)
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !fh.Quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "404", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		// Prevent deletion of directories
		if fileInfo.IsDir() {
			http.Error(w, "Cannot delete directories", http.StatusForbidden)
			if !fh.Quite {
				log.Printf("[%s] Attempt to delete directory blocked: %s\n", time.Now().Format("2006-01-02 15:04:05"), fullFilePath)
			}
			return
		}

		err = os.Remove(fullFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("[%s] Error removing file: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}

		if !fh.Quite {
			log.Printf("[%s] File deleted: %s\n", time.Now().Format("2006-01-02 15:04:05"), fullFilePath)
		}

		if encodedFilePath == "" {
			http.Redirect(w, r, "/", http.StatusSeeOther)
		} else {
			dirPath, _ := filepath.Split(string(decodedFilePath))
			encodedDirPath := base64.StdEncoding.EncodeToString([]byte(dirPath))
			http.Redirect(w, r, "/?path="+encodedDirPath, http.StatusSeeOther)
		}
	}
}

// Zip creates and serves a zip archive of a directory
func (fh *FileHandlers) Zip() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentPath := r.URL.Query().Get("path")
		zipFilename, err := fh.zipFiles(currentPath)
		if err != nil {
			http.Error(w, "Unable to create zip file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(zipFilename)
		w.Header().Set("Content-Disposition", "attachment; filename=files.zip")
		w.Header().Set("Content-Type", "application/zip")
		http.ServeFile(w, r, zipFilename)
	}
}

// Search searches within text files
func (fh *FileHandlers) Search() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filePath := r.URL.Query().Get("path")
		searchTerm := r.URL.Query().Get("term")
		caseSensitive := r.URL.Query().Get("caseSensitive") == "true"
		wholeWord := r.URL.Query().Get("wholeWord") == "true"

		log.Printf("Búsqueda - Path: %s, Término: %s, CaseSensitive: %v, WholeWord: %v",
			filePath, searchTerm, caseSensitive, wholeWord)

		if filePath == "" || searchTerm == "" {
			http.Error(w, "Missing required parameters", http.StatusBadRequest)
			return
		}

		decodedPath, err := base64.StdEncoding.DecodeString(filePath)
		if err != nil {
			http.Error(w, "Invalid path encoding", http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(fh.Dir, string(decodedPath))

		isSafe, err := security.IsSafePath(fh.Dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Invalid file path", http.StatusForbidden)
			return
		}

		results, err := utils.SearchInFile(fullPath, searchTerm, caseSensitive, wholeWord)
		if err != nil {
			http.Error(w, fmt.Sprintf("Search error: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(results)
	}
}

// handleGetRequest handles GET requests for file listing
func (fh *FileHandlers) handleGetRequest(w http.ResponseWriter, _ *http.Request, dir string, currentPath string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "The path does not exists", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	table, err := fh.createTable(files, dir, currentPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backButton := templates.CreateBackButton(currentPath)
	downloadButton := templates.CreateZipButton(currentPath)
	w.Write([]byte(statics.GetTemplates(table, backButton, downloadButton, fh.DisableHiddenFiles, fh.ReadOnly)))
}

// handlePostRequest handles file upload
func (fh *FileHandlers) handlePostRequest(w http.ResponseWriter, r *http.Request, dir string, currentPath string) {
	// Check if readonly mode is enabled
	if fh.ReadOnly {
		http.Error(w, "Upload operation is disabled in readonly mode", http.StatusForbidden)
		if !fh.Quite {
			log.Printf("[%s] Upload attempt blocked (readonly mode)\n", time.Now().Format("2006-01-02 15:04:05"))
		}
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	targetPath := filepath.Join(dir, filename)
	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0666)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer targetFile.Close()

	_, err = io.Copy(targetFile, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Redirect back to the current directory
	if currentPath == "" {
		http.Redirect(w, r, "/", http.StatusSeeOther)
	} else {
		http.Redirect(w, r, "/?path="+currentPath, http.StatusSeeOther)
	}
}

// createTable creates HTML table for file listing
func (fh *FileHandlers) createTable(files []fs.DirEntry, dir string, currentPath string) (string, error) {
	table := ""
	for _, file := range files {
		if file.Name()[0] == '.' && (!*fh.ShowHiddenFiles || fh.DisableHiddenFiles) {
			continue
		}

		fileName := file.Name()
		filePath := filepath.Join(dir, fileName)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return "", err
		}
		if file.IsDir() {
			table += templates.CreateFolderRow(file, currentPath, fileInfo)
		} else {
			fh.CustomPathsMutex.RLock()
			customPathsCopy := make(map[string]string)
			for k, v := range *fh.CustomPaths {
				customPathsCopy[k] = v
			}
			fh.CustomPathsMutex.RUnlock()
			table += templates.CreateFileRow(file, currentPath, fileInfo, customPathsCopy, fh.ReadOnly, utils.FormatFileSize)
		}
	}
	return table, nil
}

// zipFiles creates a zip file of the specified directory
func (fh *FileHandlers) zipFiles(currentPath string) (string, error) {
	decodedPath, _ := base64.StdEncoding.DecodeString(currentPath)
	fullPath := filepath.Join(fh.Dir, string(decodedPath))

	tempFile, err := os.CreateTemp(os.TempDir(), "prefix-*.zip")
	if err != nil {
		return "", err
	}
	filename := tempFile.Name()

	defer func() {
		tempFile.Close()
	}()

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	err = filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return nil
			}
			return err
		}

		if fh.DisableHiddenFiles && strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		relPath, err := filepath.Rel(fullPath, path)
		if err != nil {
			return err
		}
		header.Name = relPath

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return err
		}

		if !info.IsDir() {
			file, err := os.Open(path)
			if err != nil {
				return err
			}
			defer file.Close()
			_, err = io.Copy(writer, file)
			if err != nil {
				return err
			}
		}

		return nil
	})

	return filename, err
}
