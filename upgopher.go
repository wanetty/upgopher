package main

import (
	"archive/zip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/security"
	"github.com/wanetty/upgopher/internal/server"
	"github.com/wanetty/upgopher/internal/statics"
	"github.com/wanetty/upgopher/internal/templates"
	"github.com/wanetty/upgopher/internal/utils"
)

//go:embed static/favicon.ico
var favicon embed.FS

//go:embed static/logopher.webp
var logo embed.FS

// global vars
var quite bool = false
var version = "1.12.0"
var showHiddenFiles bool = false
var disableHiddenFiles bool = false
var readOnly bool = false
var sharedClipboard string = ""
var clipboardMutex sync.Mutex

type CustomPath struct {
	OriginalPath string
	CustomPath   string
}

var customPaths = make(map[string]string) // map[originalPath]customPath
var customPathsMutex sync.RWMutex         // protects customPaths from concurrent access

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
	readOnlyarg := flag.Bool("readonly", false, "readonly mode (disable upload and delete operations)")
	flag.Parse()
	quite = *quitearg
	readOnly = *readOnlyarg

	if !quite {
		log.Printf("Executing version %s", version)
		if readOnly {
			log.Printf("Running in READONLY mode - uploads and deletions are disabled")
		}
	}

	if _, err := os.Stat(*dir); os.IsNotExist(err) {
		os.MkdirAll(*dir, 0755)
	}

	if (*user != "" && *pass == "") || (*user == "" && *pass != "") {
		log.Fatalf("If you use the username or password you have to use both.")
		return
	}
	if *disableHiddenFilesarg {
		disableHiddenFiles = true
	}

	// Setup all routes using centralized router
	server.SetupRoutes(
		*dir,
		*user,
		*pass,
		quite,
		disableHiddenFiles,
		readOnly,
		&showHiddenFiles,
		&customPaths,
		&customPathsMutex,
		&sharedClipboard,
		&clipboardMutex,
		&favicon,
		&logo,
	)

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
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		if !quite {
			log.Printf("[%s] Starting HTTPS server on %s", time.Now().Format("2006-01-02 15:04:05"), addr)
		}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Error starting HTTPS server: %v", err)
		}
	} else {
		server := &http.Server{
			Addr:         addr,
			ReadTimeout:  60 * time.Second,
			WriteTimeout: 60 * time.Second,
			IdleTimeout:  120 * time.Second,
		}

		if !quite {
			log.Printf("[%s] Starting HTTP server on %s", time.Now().Format("2006-01-02 15:04:05"), addr)
		}
		if err := server.ListenAndServe(); err != nil {
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
		isSafe, err := security.IsSafePath(dir, newdir)
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
	backButton := templates.CreateBackButton(currentPath)
	downloadButton := templates.CreateZipButton(currentPath)
	w.Write([]byte(statics.GetTemplates(table, backButton, downloadButton, disableHiddenFiles, readOnly)))
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
			isSafe, err := security.IsSafePath(dir, fullOriginalPath)
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
			table += templates.CreateFolderRow(file, currentPath, fileInfo)
		} else {
			customPathsMutex.RLock()
			table += templates.CreateFileRow(file, currentPath, fileInfo, customPaths, readOnly, utils.FormatFileSize)
			customPathsMutex.RUnlock()
		}
	}
	return table, nil
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

// searchInFile is now in utils.SearchInFile package

// SearchResult is now defined in internal/utils package
type SearchResult = utils.SearchResult

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
		isSafe, err := security.IsSafePath(dir, fullPath)
		if err != nil || !isSafe {
			http.Error(w, "Bad path", http.StatusForbidden)
			return
		}
		// Verificar que el archivo existe y se puede leer
		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) {
			log.Printf("File not found: %s", fullPath)
			http.Error(w, fmt.Sprintf("File not found: %s", filepath.Base(fullPath)), http.StatusNotFound)
			return
		} else if err != nil {
			log.Printf("Error accessing file: %v, path: %s", err, fullPath)
			http.Error(w, "Error accessing file", http.StatusInternalServerError)
			return
		}

		// Verificar que es un archivo regular (no un directorio)
		if fileInfo.IsDir() {
			http.Error(w, "Cannot search in directory", http.StatusBadRequest)
			return
		} // Implementación real de búsqueda en archivos
		results, err := utils.SearchInFile(fullPath, searchTerm, caseSensitive, wholeWord)
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
