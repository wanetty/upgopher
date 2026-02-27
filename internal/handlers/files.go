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
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
	"github.com/wanetty/upgopher/internal/statics"
	"github.com/wanetty/upgopher/internal/templates"
	"github.com/wanetty/upgopher/internal/utils"
)

var validFolderName = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// FileHandlers manages file-related HTTP handlers
type FileHandlers struct {
	Dir                string
	Quiet              bool
	DisableHiddenFiles bool
	ReadOnly           bool
	ShowHiddenFiles    *bool
	CustomPaths        *map[string]string
	CustomPathsMutex   *sync.RWMutex
}

// NewFileHandlers creates a new FileHandlers instance
func NewFileHandlers(dir string, quiet bool, disableHiddenFiles bool, readOnly bool, showHiddenFiles *bool, customPaths *map[string]string, customPathsMutex *sync.RWMutex) *FileHandlers {
	return &FileHandlers{
		Dir:                dir,
		Quiet:              quiet,
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
		if !fh.Quiet {
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
			if !fh.Quiet {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) || fileInfo.IsDir() {
			http.Error(w, "File not found", http.StatusNotFound)
			return_code = "404"
			if !fh.Quiet {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}
		if !fh.Quiet {
			log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
		}
		http.ServeFile(w, r, fullPath)
	}
}

// Download serves files with attachment header
func (fh *FileHandlers) Download() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !fh.Quiet {
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
			if !fh.Quiet {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !fh.Quiet {
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
		if fh.ReadOnly {
			http.Error(w, "Delete operation is disabled in readonly mode", http.StatusForbidden)
			if !fh.Quiet {
				log.Printf("[%s] Delete attempt blocked (readonly mode): %s\n", time.Now().Format("2006-01-02 15:04:05"), r.URL.String())
			}
			return
		}

		if !fh.Quiet {
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
			if !fh.Quiet {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullFilePath)
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !fh.Quiet {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "404", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		// Only allow deleting empty directories
		if fileInfo.IsDir() {
			entries, readErr := os.ReadDir(fullFilePath)
			if readErr != nil {
				http.Error(w, "Cannot read directory", http.StatusInternalServerError)
				log.Printf("[%s] Error reading directory: %v\n", time.Now().Format("2006-01-02 15:04:05"), readErr)
				return
			}
			if len(entries) > 0 {
				http.Error(w, "Directory is not empty", http.StatusForbidden)
				if !fh.Quiet {
					log.Printf("[%s] Attempt to delete non-empty directory blocked: %s\n", time.Now().Format("2006-01-02 15:04:05"), fullFilePath)
				}
				return
			}
		}

		err = os.Remove(fullFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("[%s] Error removing file: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}

		if !fh.Quiet {
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

// Mkdir creates a new directory inside the shared directory
func (fh *FileHandlers) Mkdir() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if fh.ReadOnly {
			http.Error(w, "Create directory operation is disabled in readonly mode", http.StatusForbidden)
			if !fh.Quiet {
				log.Printf("[%s] Mkdir attempt blocked (readonly mode): %s\n", time.Now().Format("2006-01-02 15:04:05"), r.RemoteAddr)
			}
			return
		}

		if !fh.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		folderName := r.FormValue("folderName")
		encodedCurrentPath := r.FormValue("currentPath")

		if !validFolderName.MatchString(folderName) {
			http.Error(w, "Invalid folder name: only letters, digits, hyphens and underscores are allowed", http.StatusBadRequest)
			if !fh.Quiet {
				log.Printf("[%s] Invalid folder name rejected: %q %s\n", time.Now().Format("2006-01-02 15:04:05"), folderName, r.RemoteAddr)
			}
			return
		}

		var currentRelPath string
		if encodedCurrentPath != "" {
			decodedPath, err := base64.StdEncoding.DecodeString(encodedCurrentPath)
			if err != nil {
				http.Error(w, "Invalid path encoding", http.StatusBadRequest)
				return
			}
			currentRelPath = string(decodedPath)
		}

		// Use only the base component of folderName (belt-and-suspenders against slashes)
		safeName := filepath.Base(folderName)
		newDirPath := filepath.Join(fh.Dir, currentRelPath, safeName)

		isSafe, err := security.IsSafePath(fh.Dir, newDirPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !fh.Quiet {
				log.Printf("[%s] [POST - 403] /mkdir %s\n", time.Now().Format("2006-01-02 15:04:05"), r.RemoteAddr)
			}
			return
		}

		if err := os.Mkdir(newDirPath, 0755); err != nil {
			if os.IsExist(err) {
				http.Error(w, "Directory already exists", http.StatusConflict)
			} else {
				http.Error(w, "Failed to create directory", http.StatusInternalServerError)
				log.Printf("[%s] Error creating directory: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			}
			return
		}

		if !fh.Quiet {
			log.Printf("[%s] Directory created: %s\n", time.Now().Format("2006-01-02 15:04:05"), newDirPath)
		}

		w.WriteHeader(http.StatusCreated)
	}
}

// Zip creates and serves a zip archive of a directory
func (fh *FileHandlers) Zip() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentPath := r.URL.Query().Get("path")

		if currentPath != "" {
			decodedPath, err := base64.StdEncoding.DecodeString(currentPath)
			if err != nil {
				http.Error(w, "Invalid path encoding", http.StatusBadRequest)
				return
			}
			fullPath := filepath.Join(fh.Dir, string(decodedPath))
			isSafe, err := security.IsSafePath(fh.Dir, fullPath)
			if err != nil || !isSafe {
				http.Error(w, "Bad path", http.StatusForbidden)
				return
			}
		}

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
		if !fh.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		filePath := r.URL.Query().Get("path")
		searchTerm := r.URL.Query().Get("term")
		caseSensitive := r.URL.Query().Get("caseSensitive") == "true"
		wholeWord := r.URL.Query().Get("wholeWord") == "true"

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

// FileContent serves the text content of a file as JSON for in-browser viewing
func (fh *FileHandlers) FileContent() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !fh.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		encodedPath := r.URL.Query().Get("path")
		if encodedPath == "" {
			http.Error(w, "Missing path parameter", http.StatusBadRequest)
			return
		}

		decodedPath, err := base64.StdEncoding.DecodeString(encodedPath)
		if err != nil {
			http.Error(w, "Invalid path encoding", http.StatusBadRequest)
			return
		}

		fullPath := filepath.Join(fh.Dir, string(decodedPath))
		isSafe, err := security.IsSafePath(fh.Dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}
		if fileInfo.IsDir() {
			http.Error(w, "Path is a directory", http.StatusBadRequest)
			return
		}

		_, filename := filepath.Split(fullPath)
		if !templates.IsTextFile(filename) {
			http.Error(w, "File type not supported for viewing", http.StatusUnsupportedMediaType)
			return
		}

		const maxFileSize = 1 << 20 // 1 MB
		if fileInfo.Size() > maxFileSize {
			http.Error(w, "File too large to view (max 1 MB)", http.StatusRequestEntityTooLarge)
			return
		}

		content, err := os.ReadFile(fullPath)
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{
			"filename": filename,
			"content":  string(content),
		})
	}
}

// ZipSelected creates and serves a zip archive of specifically selected files
func (fh *FileHandlers) ZipSelected() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if !fh.Quiet {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		var req struct {
			Paths []string `json:"paths"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}

		if len(req.Paths) == 0 {
			http.Error(w, "No files selected", http.StatusBadRequest)
			return
		}

		// Decode and validate each path individually — one bad path aborts all
		fullPaths := make([]string, 0, len(req.Paths))
		for _, encodedPath := range req.Paths {
			decoded, err := base64.StdEncoding.DecodeString(encodedPath)
			if err != nil {
				http.Error(w, "Invalid path encoding", http.StatusBadRequest)
				return
			}
			fullPath := filepath.Join(fh.Dir, string(decoded))
			isSafe, err := security.IsSafePath(fh.Dir, fullPath)
			if err != nil || !isSafe {
				http.Error(w, "Bad path", http.StatusForbidden)
				return
			}
			info, err := os.Stat(fullPath)
			if err != nil || info.IsDir() {
				continue
			}
			fullPaths = append(fullPaths, fullPath)
		}

		if len(fullPaths) == 0 {
			http.Error(w, "No valid files to zip", http.StatusBadRequest)
			return
		}

		zipFilename, err := fh.zipSpecificFiles(fullPaths)
		if err != nil {
			http.Error(w, "Unable to create zip file", http.StatusInternalServerError)
			return
		}
		defer os.Remove(zipFilename)
		w.Header().Set("Content-Disposition", "attachment; filename=selected-files.zip")
		w.Header().Set("Content-Type", "application/zip")
		http.ServeFile(w, r, zipFilename)
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
	if fh.ReadOnly {
		http.Error(w, "Upload operation is disabled in readonly mode", http.StatusForbidden)
		if !fh.Quiet {
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

	// Strip any directory components from the filename to prevent path traversal
	filename := filepath.Base(header.Filename)
	targetPath := filepath.Join(dir, filename)
	isSafe, err := security.IsSafePath(dir, targetPath)
	if err != nil || !isSafe {
		http.Error(w, "Bad path", http.StatusForbidden)
		return
	}

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
			table += templates.CreateFolderRow(file, currentPath, fileInfo, fh.ReadOnly)
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

// zipSpecificFiles creates a zip archive containing specifically selected files
func (fh *FileHandlers) zipSpecificFiles(fullPaths []string) (string, error) {
	tempFile, err := os.CreateTemp(os.TempDir(), "selected-*.zip")
	if err != nil {
		return "", err
	}
	filename := tempFile.Name()
	defer tempFile.Close()

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	for _, fullPath := range fullPaths {
		info, err := os.Stat(fullPath)
		if err != nil {
			continue
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return "", err
		}

		relPath, err := filepath.Rel(fh.Dir, fullPath)
		if err != nil {
			header.Name = info.Name()
		} else {
			header.Name = relPath
		}
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			return "", err
		}

		file, err := os.Open(fullPath)
		if err != nil {
			return "", err
		}
		_, copyErr := io.Copy(writer, file)
		file.Close()
		if copyErr != nil {
			return "", copyErr
		}
	}

	return filename, nil
}
