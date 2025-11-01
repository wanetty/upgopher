package main

import (
	"archive/zip"
	"bufio"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/statics"
)

//go:embed static/favicon.ico
var favicon embed.FS

//go:embed static/logopher.webp
var logo embed.FS

// global vars
var quite bool = false
var version = "1.10.1"
var showHiddenFiles bool = false
var disableHiddenFiles bool = false
var sharedClipboard string = ""

type CustomPath struct {
	OriginalPath string
	CustomPath   string
}

var customPaths = make(map[string]string) // map[originalPath]customPath
var customPathsMutex sync.RWMutex         // protects customPaths from concurrent access

// Rate limiting for clipboard endpoint
var clipboardRateLimiter sync.Map // map[string][]time.Time - IP -> request timestamps

// Handlers //////////////////////////////////////////////////
func fileHandlerWithDir(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		fileHandler(w, r, dir)
	}
}
func rawHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		return_code := "200"
		path := strings.TrimPrefix(r.URL.Path, "/raw/")
		fullPath := filepath.Join(dir, path)

		isSafe, err := isSafePath(dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			return_code = "403"
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) || fileInfo.IsDir() {
			http.Error(w, "File not found", http.StatusNotFound)
			return_code = "404"
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}
		if !quite {
			log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, return_code, r.URL.Path, r.RemoteAddr)
		}
		http.ServeFile(w, r, fullPath)
	}
}

func uploadHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		encodedFilePath := r.URL.Query().Get("path")
		decodedFilePath, err := base64.StdEncoding.DecodeString(encodedFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		fullFilePath := filepath.Join(dir, string(decodedFilePath))
		isSafe, err := isSafePath(dir, fullFilePath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "404", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		_, filename := filepath.Split(fullFilePath)
		w.Header().Set("Content-Disposition", "attachment; filename="+filename)
		http.ServeFile(w, r, fullFilePath)
	}
}

func deleteHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !quite {
			log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
		}

		encodedFilePath := r.URL.Query().Get("path")
		decodedFilePath, err := base64.StdEncoding.DecodeString(encodedFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("[%s] Error decoding path: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}

		fullFilePath := filepath.Join(dir, string(decodedFilePath))
		isSafe, err := isSafePath(dir, fullFilePath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "404", r.URL.Path, r.RemoteAddr)
			}
			return
		}

		err = os.Remove(fullFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			log.Printf("[%s] Error removing file: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}

		if !quite {
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

func zipHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		currentPath := r.URL.Query().Get("path")
		zipFilename, err := ZipFiles(dir, currentPath)
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

func showHiddenFilesHandler(w http.ResponseWriter, r *http.Request) {
	// Handle GET request - return current hidden files status
	if r.Method == http.MethodGet {
		if !quite {
			log.Printf("[%s] Getting hidden files setting: %t\n", time.Now().Format("2006-01-02 15:04:05"), showHiddenFiles)
		}
		if showHiddenFiles {
			w.Write([]byte("true"))
			return
		} else {
			w.Write([]byte("false"))
		}
	} else if r.Method == http.MethodPost {
		// Handle POST request - toggle hidden files setting
		if !quite {
			log.Printf("[%s] Toggling hidden files setting\n", time.Now().Format("2006-01-02 15:04:05"))
		}
		if disableHiddenFiles {
			http.Error(w, "You can't change this setting", http.StatusForbidden)
			return
		} else {
			showHiddenFiles = !showHiddenFiles
			return
		}
	}
}

func faviconHandler(w http.ResponseWriter, r *http.Request) {
	faviconData, err := favicon.ReadFile("static/favicon.ico")
	if err != nil {
		http.Error(w, "Favicon not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/x-icon")
	w.Write(faviconData)
}

func logoHandler(w http.ResponseWriter, r *http.Request) {
	logoData, err := logo.ReadFile("static/logopher.webp")
	if err != nil {
		http.Error(w, "Logo not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/png")
	w.Write(logoData)
}

func applyBasicAuth(handler http.HandlerFunc, user, pass string) http.HandlerFunc {
	userByte := []byte(user)
	passByte := []byte(pass)
	return basicAuth(handler, userByte, passByte)
}

// checkRateLimit checks if the IP has exceeded rate limit (20 requests per minute)
func checkRateLimit(ip string) bool {
	now := time.Now()
	oneMinuteAgo := now.Add(-time.Minute)

	// Get or create timestamp list for this IP
	value, _ := clipboardRateLimiter.LoadOrStore(ip, []time.Time{})
	timestamps := value.([]time.Time)

	// Filter out timestamps older than 1 minute
	var recentRequests []time.Time
	for _, ts := range timestamps {
		if ts.After(oneMinuteAgo) {
			recentRequests = append(recentRequests, ts)
		}
	}

	// Check if rate limit exceeded
	if len(recentRequests) >= 20 {
		return false // rate limit exceeded
	}

	// Add current request timestamp
	recentRequests = append(recentRequests, now)
	clipboardRateLimiter.Store(ip, recentRequests)

	return true // request allowed
}

// Clipboard handler to get and update shared clipboard content
func clipboardHandler(w http.ResponseWriter, r *http.Request) {
	if !quite {
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
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(sharedClipboard))
		if !quite {
			log.Printf("[%s] Clipboard content returned: '%s'\n", time.Now().Format("2006-01-02 15:04:05"), sharedClipboard)
		}
	} else if r.Method == http.MethodPost {
		// Check rate limit before processing POST request
		clientIP := r.RemoteAddr
		if !checkRateLimit(clientIP) {
			http.Error(w, "Rate limit exceeded. Maximum 20 requests per minute.", http.StatusTooManyRequests)
			if !quite {
				log.Printf("[%s] Rate limit exceeded for IP: %s\n", time.Now().Format("2006-01-02 15:04:05"), clientIP)
			}
			return
		}

		// Update clipboard with received data
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Error reading data", http.StatusBadRequest)
			log.Printf("[%s] Error reading clipboard data: %v\n", time.Now().Format("2006-01-02 15:04:05"), err)
			return
		}
		defer r.Body.Close()

		sharedClipboard = string(body)
		w.WriteHeader(http.StatusOK)
		if !quite {
			log.Printf("[%s] Clipboard updated to: '%s'\n", time.Now().Format("2006-01-02 15:04:05"), sharedClipboard)
		}
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// Main /////////////////////////////////////////////////
func main() {
	port := flag.Int("port", 9090, "port number")
	dir := flag.String("dir", "./uploads", "directory path")
	user := flag.String("user", "", "username for authentication")
	pass := flag.String("pass", "", "password for authentication")
	useTLS := flag.Bool("ssl", false, "use HTTPS on port 443 by default. (If you don't put cert and key, it will generate a self-signed certificate)")
	certFile := flag.String("cert", "", "HTTPS certificate")
	keyFile := flag.String("key", "", "private key for HTTPS")
	quitearg := flag.Bool("q", false, "quite mode")
	disableHiddenFilesarg := flag.Bool("disable-hidden-files", false, "disable showing hidden files")
	flag.Parse()
	quite = *quitearg

	if !quite {
		log.Printf("Executing version %s", version)
	}

	if _, err := os.Stat(*dir); os.IsNotExist(err) {
		os.MkdirAll(*dir, 0755)
	}

	fileHandler := fileHandlerWithDir(*dir)
	uploadHandler := uploadHandler(*dir)
	deleteHandler := deleteHandler(*dir)
	rawHandler := rawHandler(*dir)
	zipHandler := zipHandler(*dir)

	if (*user != "" && *pass == "") || (*user == "" && *pass != "") {
		log.Fatalf("If you use the username or password you have to use both.")
		return
	}
	if *disableHiddenFilesarg {
		disableHiddenFiles = true
	}
	if *user != "" && *pass != "" {
		http.HandleFunc("/", applyBasicAuth(fileHandler, *user, *pass))
		http.Handle("/delete/", http.StripPrefix("/delete/", applyBasicAuth(deleteHandler, *user, *pass)))
		http.Handle("/download/", http.StripPrefix("/download/", applyBasicAuth(uploadHandler, *user, *pass)))
		http.Handle("/raw/", http.StripPrefix("/raw/", applyBasicAuth(rawHandler, *user, *pass)))
		http.HandleFunc("/favicon.ico", applyBasicAuth(faviconHandler, *user, *pass))
		http.HandleFunc("/static/logopher.webp", applyBasicAuth(logoHandler, *user, *pass))
		http.HandleFunc("/zip", applyBasicAuth(zipHandler, *user, *pass))
		http.HandleFunc("/showhiddenfiles", applyBasicAuth(showHiddenFilesHandler, *user, *pass))
		http.HandleFunc("/custom-path", applyBasicAuth(createCustomPathHandler(*dir), *user, *pass))
		http.HandleFunc("/clipboard", applyBasicAuth(clipboardHandler, *user, *pass))
		http.HandleFunc("/search-file", applyBasicAuth(searchFileHandler(*dir), *user, *pass))
	} else {
		http.HandleFunc("/", fileHandler)
		http.Handle("/delete/", http.StripPrefix("/delete/", deleteHandler))
		http.Handle("/download/", http.StripPrefix("/download/", uploadHandler))
		http.Handle("/raw/", http.StripPrefix("/raw/", rawHandler))
		http.HandleFunc("/favicon.ico", faviconHandler)
		http.HandleFunc("/static/logopher.webp", logoHandler)
		http.HandleFunc("/zip", zipHandler)
		http.HandleFunc("/showhiddenfiles", showHiddenFilesHandler)
		http.HandleFunc("/custom-path", createCustomPathHandler(*dir))
		http.HandleFunc("/clipboard", clipboardHandler)
		http.HandleFunc("/search-file", searchFileHandler(*dir))
	}

	if !isFlagPassed("port") && *useTLS {
		*port = 443
	}
	addr := fmt.Sprintf("0.0.0.0:%d", *port)
	startServer(addr, *useTLS, *certFile, *keyFile, *port)
}

func startServer(addr string, useTLS bool, certFile, keyFile string, _ int) {
	if useTLS {
		var cert tls.Certificate
		var err error

		if certFile != "" && keyFile != "" {
			cert, err = tls.LoadX509KeyPair(certFile, keyFile)
			if err != nil {
				log.Fatalf("Failed to load certificate and key pair: %v", err)
			}
		} else {
			log.Println("No certificate or key file provided, generating a self-signed certificate.")
			certPEM, keyPEM, err := generateSelfSignedCert()
			if err != nil {
				log.Fatalf("Failed to generate self-signed certificate: %v", err)
			}

			cert, err = tls.X509KeyPair(certPEM, keyPEM)
			if err != nil {
				log.Fatalf("Failed to create key pair from generated self-signed certificate: %v", err)
			}
		}

		server := &http.Server{
			Addr: addr,
			TLSConfig: &tls.Config{
				Certificates: []tls.Certificate{cert},
			},
		}

		if !quite {
			log.Printf("[%s] Starting HTTPS server on %s", time.Now().Format("2006-01-02 15:04:05"), addr)
		}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Error starting HTTPS server: %v", err)
		}
	} else {
		if !quite {
			log.Printf("[%s] Starting HTTP server on %s", time.Now().Format("2006-01-02 15:04:05"), addr)
		}
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Fatalf("Error starting HTTP server: %v", err)
		}
	}
}

func generateSelfSignedCert() ([]byte, []byte, error) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, err
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Self-signed"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return nil, nil, err
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	return certPEM, keyPEM, nil
}

func isSafePath(baseDir, userPath string) (bool, error) {
	absBaseDir, err := filepath.Abs(baseDir)
	if err != nil {
		return false, err
	}

	absUserPath, err := filepath.Abs(userPath)
	if err != nil {
		return false, err
	}

	return strings.HasPrefix(absUserPath, absBaseDir), nil
}

func basicAuth(handler http.HandlerFunc, username, password []byte) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user, pass, ok := r.BasicAuth()
		if !ok || subtle.ConstantTimeCompare([]byte(user), username) != 1 || subtle.ConstantTimeCompare([]byte(pass), password) != 1 {
			w.Header().Set("WWW-Authenticate", `Basic realm="Restricted"`)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte("Unauthorized.\n"))
			return
		}
		handler(w, r)
	}
}

func fileHandler(w http.ResponseWriter, r *http.Request, dir string) {
	if !quite {
		log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)
	}

	// Check if it's a custom path
	requestPath := strings.TrimPrefix(r.URL.Path, "/")
	for originalPath, customPath := range customPaths {
		if customPath == requestPath {
			encodedPath := base64.StdEncoding.EncodeToString([]byte(originalPath))
			http.Redirect(w, r, "/download/?path="+encodedPath, http.StatusSeeOther)
			return
		}
	}

	currentPath := r.URL.Query().Get("path")
	if currentPath != "" {
		decodedPath, err := base64.StdEncoding.DecodeString(currentPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		newdir := filepath.Join(dir, string(decodedPath))
		isSafe, err := isSafePath(dir, newdir)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			if !quite {
				log.Printf("[%s] [%s - %s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, "403", r.URL.Path, r.RemoteAddr)
			}
			return
		} else {
			dir = newdir
		}
	}
	if r.Method == http.MethodPost {
		handlePostRequest(w, r, dir)
	} else {
		handleGetRequest(w, r, dir, currentPath)
	}
}

func handlePostRequest(w http.ResponseWriter, r *http.Request, dir string) {
	file, header, err := r.FormFile("file")
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	defer file.Close()

	filename := header.Filename
	targetPath := filepath.Join(dir, filename)
	targetFile, err := os.OpenFile(targetPath, os.O_WRONLY|os.O_CREATE, 0666)

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
	http.Redirect(w, r, r.URL.String(), http.StatusSeeOther)
}

func handleGetRequest(w http.ResponseWriter, _ *http.Request, dir string, currentPath string) {
	files, err := os.ReadDir(dir)
	if err != nil {
		http.Error(w, "The path does not exists", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	table, err := createTable(files, dir, currentPath)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	backButton := createBackButton(currentPath)
	downloadButton := createZipButton(currentPath)
	w.Write([]byte(statics.GetTemplates(table, backButton, downloadButton, disableHiddenFiles)))
}

func createCustomPathHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			originalPath := r.FormValue("originalPath")
			customPath := r.FormValue("customPath")

			// Verificar si el custom path ya existe
			customPathsMutex.RLock()
			for _, existingCustomPath := range customPaths {
				if existingCustomPath == customPath {
					customPathsMutex.RUnlock()
					http.Error(w, "Custom path already exists", http.StatusConflict)
					return
				}
			}
			customPathsMutex.RUnlock()

			// Validar paths
			fullOriginalPath := filepath.Join(dir, originalPath)
			isSafe, err := isSafePath(dir, fullOriginalPath)
			if err != nil || !isSafe {
				http.Error(w, "Invalid original path", http.StatusBadRequest)
				return
			}

			// Asegurarse de que el archivo existe
			if _, err := os.Stat(fullOriginalPath); os.IsNotExist(err) {
				http.Error(w, "Original file does not exist", http.StatusBadRequest)
				return
			}

			// Almacenar el custom path
			customPathsMutex.Lock()
			customPaths[originalPath] = customPath
			customPathsMutex.Unlock()
			w.WriteHeader(http.StatusOK)
			return
		}

		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func createTable(files []fs.DirEntry, dir string, currentPath string) (string, error) {
	table := ""
	for _, file := range files {
		if file.Name()[0] == '.' && (!showHiddenFiles || disableHiddenFiles) {
			continue
		}

		fileName := file.Name()
		filePath := filepath.Join(dir, fileName)
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			return "", err
		}
		if file.IsDir() {
			table += createFolderRow(file, currentPath, fileInfo)
		} else {
			table += createFileRow(file, currentPath, fileInfo)
		}
	}
	return table, nil
}

func createZipButton(currentPath string) string {
	if currentPath != "" {
		return `<button class="btn" onclick="window.location.href='/zip?path=` + currentPath + `'"><i class="fa fa-download"></i> Download Zip</button>`
	} else {
		return `<button class="btn" onclick="window.location.href='/zip'"><i class="fa fa-download"></i> Download Zip</button>`
	}
}

func createFolderRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo) string {
	encodedPath := createEncodedPath(currentPath, file.Name())
	escapedencodedFilePath := html.EscapeString(encodedPath)

	escapedFolderName := html.EscapeString(file.Name())
	folderLink := fmt.Sprintf(`<a href="/?path=%s">%s</a>`, escapedencodedFilePath, escapedFolderName)
	lastModified := fileInfo.ModTime().Format("2006-01-02 15:04:05")
	return fmt.Sprintf(`
		<tr>
			<td>%s</td>
			<td>%s</td>
			<td>-</td>
			<td>%s</td>
			<td>-</td>
			<td>
				<div class="action-buttons">
					<span>-</span>
				</div>
			</td>
		</tr>
	`, folderLink, fileInfo.Mode(), lastModified)
}

func createFileRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo) string {
	encodedFilePath := createEncodedPath(currentPath, file.Name())

	escapedFileName := html.EscapeString(file.Name())
	escapedencodedFilePath := html.EscapeString(encodedFilePath)

	//decodeamos path
	decodedPath, _ := base64.StdEncoding.DecodeString(currentPath)

	// Buscar custom path para este archivo
	customPathDisplay := "-"
	filePath := filepath.Join(string(decodedPath), file.Name())
	customPathsMutex.RLock()
	customPath, exists := customPaths[filePath]
	customPathsMutex.RUnlock()
	fmt.Println(currentPath)
	if exists {
		customPathDisplay = html.EscapeString(customPath)
	}
	// Determinar si el archivo es legible (texto)
	isReadableFile := isTextFile(file.Name())

	// Usar action-buttons y los estilos de botones adecuados
	downloadLink := fmt.Sprintf(`<button class="action-btn download" title="Download" onclick="window.location.href='/download/?path=%s'"><i class="fa fa-download"></i></button>`, escapedencodedFilePath)
	deleteLink := fmt.Sprintf(`<button class="action-btn delete" title="Delete" onclick="window.location.href='/delete/?path=%s'"><i class="fa fa-trash"></i></button>`, escapedencodedFilePath)
	copyURLButton := fmt.Sprintf(`<button class="action-btn link" title="Copy URL" onclick="copyToClipboard('%s', '%s')"><i class="fa fa-link"></i></button>`, currentPath, escapedFileName)
	customPathButton := fmt.Sprintf(`<button class="action-btn edit" title="Create Custom Path" onclick="showCustomPathForm('%s', '%s')"><i class="fa fa-magic"></i></button>`, escapedFileName, currentPath)
	// Botón de búsqueda solo para archivos legibles
	searchButton := ""
	if isReadableFile {
		searchButton = fmt.Sprintf(`<button class="action-btn search" title="Search in File" onclick="showSearchModal('%s', '%s')"><i class="fa fa-search"></i></button>`, escapedencodedFilePath, escapedFileName)
	}

	fileSize, units := formatFileSize(fileInfo.Size())
	lastModified := fileInfo.ModTime().Format("2006-01-02 15:04:05")

	return fmt.Sprintf(`
		<tr>
			<td>%s</td>
			<td>%s</td>
			<td>%.2f %s</td>
			<td>%s</td>
			<td>%s</td>
			<td>
				<div class="action-buttons">
					%s%s%s%s%s
				</div>
			</td>
		</tr>
	`, escapedFileName, fileInfo.Mode(), fileSize, units, lastModified, customPathDisplay,
		downloadLink, copyURLButton, customPathButton, searchButton, deleteLink)
}

func createEncodedPath(currentPath string, fileName string) string {
	decodedFilePath, _ := base64.StdEncoding.DecodeString(currentPath)
	return base64.StdEncoding.EncodeToString([]byte(filepath.Join(string(decodedFilePath), fileName)))
}

func formatFileSize(size int64) (float64, string) {
	if size < 1000 {
		return float64(size), "bytes"
	} else if size < 1000000 {
		return float64(size) / 1000, "KBytes"
	} else {
		return float64(size) / 1000000, "MBytes"
	}
}

func createBackButton(currentPath string) string {
	if currentPath != "" {
		return `<button class="btn" onclick="window.location.href='/'"><i class="fa fa-arrow-left"></i> Back</button>`
	}
	return ""
}
func ZipFiles(dir, currentPath string) (string, error) {
	// Decodifica la ruta actual
	decodedPath, _ := base64.StdEncoding.DecodeString(currentPath)
	fullPath := filepath.Join(dir, string(decodedPath))

	// Crea archivo temporal para el ZIP
	tempFile, err := os.CreateTemp(os.TempDir(), "prefix-*.zip")
	if err != nil {
		return "", err
	}
	filename := tempFile.Name()

	// Defer para asegurar que el archivo temporal se elimine
	defer func() {
		tempFile.Close()
	}()

	zipWriter := zip.NewWriter(tempFile)
	defer zipWriter.Close()

	// Agrega archivos y directorios al ZIP
	err = filepath.Walk(fullPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsPermission(err) {
				return nil
			}
			return err
		}

		// Ignora archivos ocultos si la opción está deshabilitada
		if disableHiddenFiles && strings.HasPrefix(info.Name(), ".") {
			return nil
		}

		// Añadir directorios al archivo ZIP
		if info.IsDir() {
			if err := addDirToZip(zipWriter, path, fullPath); err != nil {
				return err
			}
			return nil
		}

		// Añadir archivos regulares
		if info.Mode().IsRegular() {
			if err := addFileToZip(zipWriter, path, fullPath); err != nil {
				return err
			}
		}

		return nil
	})
	if err != nil {
		return "", err
	}

	return filename, nil
}

func addDirToZip(zipWriter *zip.Writer, dirPath string, basePath string) error {
	relPath, err := filepath.Rel(basePath, dirPath)
	if err != nil {
		return err
	}

	if !strings.HasSuffix(relPath, "/") {
		relPath += "/"
	}

	_, err = zipWriter.Create(relPath)
	return err
}

func addFileToZip(zipWriter *zip.Writer, filePath string, basePath string) error {
	relPath, err := filepath.Rel(basePath, filePath)
	if err != nil {
		return err
	}

	file, err := os.Open(filePath)
	if err != nil {
		return err
	}
	defer file.Close()

	wr, err := zipWriter.Create(relPath)
	if err != nil {
		return err
	}
	_, err = io.Copy(wr, file)
	return err
}

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// isTextFile determina si un archivo es probablemente un archivo de texto legible
func isTextFile(fileName string) bool {
	ext := strings.ToLower(filepath.Ext(fileName))
	// Lista de extensiones de archivo que se consideran legibles como texto
	textExtensions := map[string]bool{
		".txt": true, ".md": true, ".json": true, ".xml": true, ".html": true, ".css": true,
		".js": true, ".go": true, ".py": true, ".java": true, ".c": true, ".cpp": true,
		".h": true, ".swift": true, ".rb": true, ".php": true, ".sh": true, ".bat": true,
		".log": true, ".csv": true, ".yml": true, ".yaml": true, ".toml": true, ".ini": true,
		".cfg": true, ".conf": true, ".properties": true, ".env": true, ".sql": true,
	}
	return textExtensions[ext]
}

// searchInFile busca un término en un archivo y devuelve los resultados
func searchInFile(filePath, searchTerm string, caseSensitive, wholeWord bool) ([]SearchResult, error) {

	// Comprobar que el archivo existe antes de intentar abrirlo
	if _, err := os.Stat(filePath); os.IsNotExist(err) {
		return nil, fmt.Errorf("the file does not exist: %s", filePath)
	}

	// Abrir el archivo
	file, err := os.Open(filePath)
	if err != nil {
		log.Printf("Error to open file: %v", err)
		return nil, err
	}
	defer file.Close()

	// Crear un escáner para leer el archivo línea por línea
	scanner := bufio.NewScanner(file)
	lineNumber := 0
	var results []SearchResult

	// Preparar el término de búsqueda según las opciones
	var searchFunc func(string) bool

	if !caseSensitive {
		searchTerm = strings.ToLower(searchTerm)
	}

	if wholeWord {
		// Para búsqueda de palabra completa, usamos una expresión regular
		var pattern string
		if caseSensitive {
			pattern = fmt.Sprintf(`\b%s\b`, regexp.QuoteMeta(searchTerm))
		} else {
			pattern = fmt.Sprintf(`(?i)\b%s\b`, regexp.QuoteMeta(searchTerm))
		}

		re, err := regexp.Compile(pattern)
		if err != nil {
			return nil, err
		}

		searchFunc = func(line string) bool {
			return re.MatchString(line)
		}
	} else {
		// Para búsqueda normal
		searchFunc = func(line string) bool {
			if caseSensitive {
				return strings.Contains(line, searchTerm)
			}
			return strings.Contains(strings.ToLower(line), searchTerm)
		}
	}

	// Leer el archivo línea por línea
	for scanner.Scan() {
		lineNumber++
		line := scanner.Text()

		// Verificar si la línea contiene el término de búsqueda
		if searchFunc(line) {
			// Limitar la longitud de la línea para evitar enviar demasiados datos
			if len(line) > 300 {
				line = line[:300] + "..."
			}

			results = append(results, SearchResult{
				LineNumber: lineNumber,
				Content:    line,
			})

			// Limitar el número total de resultados para evitar problemas de rendimiento
			if len(results) >= 1000 {
				results = append(results, SearchResult{
					LineNumber: -1,
					Content:    "Search results limited to 1000 matches.",
				})
				break
			}
		}
	}
	// Verificar errores de lectura
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	// Si no encontramos resultados, devolver un mensaje en lugar de un array vacío
	if len(results) == 0 {
		results = append(results, SearchResult{
			LineNumber: -1,
			Content:    "No matches found.",
		})
	}

	return results, nil
}

// Resultado de búsqueda que se enviará al cliente
type SearchResult struct {
	LineNumber int    `json:"lineNumber"`
	Content    string `json:"content"`
}

// searchFileHandler maneja las solicitudes de búsqueda en archivos
func searchFileHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Log más detallado para depuración
		log.Printf("[%s] [%s] %s %s\n", time.Now().Format("2006-01-02 15:04:05"), r.Method, r.URL.String(), r.RemoteAddr)

		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		// Obtener los parámetros de búsqueda
		filePath := r.URL.Query().Get("path")
		searchTerm := r.URL.Query().Get("term")
		caseSensitive := r.URL.Query().Get("caseSensitive") == "true"
		wholeWord := r.URL.Query().Get("wholeWord") == "true"

		log.Printf("Búsqueda - Path: %s, Término: %s, CaseSensitive: %t, WholeWord: %t",
			filePath, searchTerm, caseSensitive, wholeWord)

		// Para evitar warnings de compilación mientras no usemos estos parámetros
		_ = caseSensitive
		_ = wholeWord

		// Validar que tenemos los parámetros necesarios
		if filePath == "" || searchTerm == "" {
			http.Error(w, "Missing required parameters", http.StatusBadRequest)
			return
		}
		decodedURL, err := url.QueryUnescape(filePath)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid URL encoding: %v", err), http.StatusBadRequest)
			return
		}

		// Ahora decodificar desde base64
		decodedFilePath, err := base64.StdEncoding.DecodeString(decodedURL)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid base64 encoding: %v", err), http.StatusBadRequest)
			return
		}

		// Construir la ruta completa al archivo
		fullPath := filepath.Join(dir, string(decodedFilePath))

		// Verificar que la ruta es segura
		isSafe, err := isSafePath(dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			return
		}
		// Verificar que el archivo existe y se puede leer
		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			log.Printf("File not found: %s", fullPath)
			http.Error(w, fmt.Sprintf("File not found: %s", fullPath), http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Error accessing file: %v, path: %s", err, fullPath)
			http.Error(w, fmt.Sprintf("Error accessing file: %v", err), http.StatusInternalServerError)
			return
		}

		// Verificar que es un archivo regular (no un directorio)
		if fileInfo.IsDir() {
			http.Error(w, "Cannot search in directory", http.StatusBadRequest)
			return
		} // Implementación real de búsqueda en archivos
		results, err := searchInFile(fullPath, searchTerm, caseSensitive, wholeWord)
		if err != nil {
			log.Printf("Error searching in file: %v", err)
			http.Error(w, "Error searching in file", http.StatusInternalServerError)
			return
		}

		// Configurar el encabezado de contenido JSON
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)

		// Serializar y enviar la respuesta
		json.NewEncoder(w).Encode(results)
	}
}
