package main

import (
	"archive/zip"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/base64"
	"encoding/pem"
	"flag"
	"fmt"
	"html"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wanetty/upgopher/internal/statics"
)

//go:embed static/favicon.ico
var favicon embed.FS

//go:embed static/logopher.webp
var logo embed.FS

// global vars
var quite bool = false
var version = "1.6.3"
var showHiddenFiles bool = false
var disableHiddenFiles bool = false

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
				log.Printf("[%s - %s] %s %s\n", r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}

		fileInfo, err := os.Stat(fullPath)
		if os.IsNotExist(err) || fileInfo.IsDir() {
			http.Error(w, "File not found", http.StatusNotFound)
			return_code = "404"
			if !quite {
				log.Printf("[%s - %s] %s %s\n", r.Method, return_code, r.URL.Path, r.RemoteAddr)
			}
			return
		}
		if !quite {
			log.Printf("[%s - %s] %s %s\n", r.Method, return_code, r.URL.Path, r.RemoteAddr)
		}
		http.ServeFile(w, r, fullPath)
	}
}

func uploadHandler(dir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !quite {
			log.Printf("[%s] %s%s %s\n", r.Method, "/download/", r.URL.String(), r.RemoteAddr)
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
				log.Printf("[%s] %s%s %s\n", r.Method, "/download/", r.URL.String(), r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
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
			log.Printf("[%s] %s%s %s\n", r.Method, "/delete/", r.URL.String(), r.RemoteAddr)
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
				log.Printf("[%s] %s%s %s\n", r.Method, "/delete/", r.URL.String(), r.RemoteAddr)
			}
			return
		}

		if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
			http.Error(w, "File not found", http.StatusNotFound)
			return
		}

		err = os.Remove(fullFilePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
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
	//if is http get request
	if r.Method == http.MethodGet {
		if showHiddenFiles {
			fmt.Fprintf(w, "true")
			return
		} else {
			fmt.Fprintf(w, "false")
		}
	} else if r.Method == http.MethodPost {
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
	} else {
		http.HandleFunc("/", fileHandler)
		http.Handle("/delete/", http.StripPrefix("/delete/", deleteHandler))
		http.Handle("/download/", http.StripPrefix("/download/", uploadHandler))
		http.Handle("/raw/", http.StripPrefix("/raw/", rawHandler))
		http.HandleFunc("/favicon.ico", faviconHandler)
		http.HandleFunc("/static/logopher.webp", logoHandler)
		http.HandleFunc("/zip", zipHandler)
		http.HandleFunc("/showhiddenfiles", showHiddenFilesHandler)
	}
	if !isFlagPassed("port") && *useTLS {
		*port = 443
	}
	addr := fmt.Sprintf(":%d", *port)
	startServer(addr, *useTLS, *certFile, *keyFile, *port)
}

func startServer(addr string, useTLS bool, certFile, keyFile string, port int) {
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
			log.Printf("Starting HTTPS server on %s", addr)
		}
		if err := server.ListenAndServeTLS("", ""); err != nil {
			log.Fatalf("Error starting HTTPS server: %v", err)
		}
	} else {
		if !quite {
			log.Printf("Starting HTTP server on %s", addr)
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
		log.Printf("[%s] %s %s\n", r.Method, r.URL.String(), r.RemoteAddr)
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
				log.Printf("[%s - %s] %s %s\n", r.Method, "403", r.URL.Path, r.RemoteAddr)
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

func handleGetRequest(w http.ResponseWriter, r *http.Request, dir string, currentPath string) {
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
	fmt.Fprintf(w, statics.GetTemplates(table, backButton, downloadButton, disableHiddenFiles))
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
		return `<a class="btn" href="/zip?path=` + currentPath + `">Download Zip</a>`
	} else {
		return `<a class="btn" href="/zip">Download Zip</a>`
	}
}

func createFolderRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo) string {
	encodedPath := createEncodedPath(currentPath, file.Name())
	escapedencodedFilePath := html.EscapeString(encodedPath)

	folderLink := fmt.Sprintf(`<a href="/?path=%s">%s</a>`, escapedencodedFilePath, file.Name())
	return fmt.Sprintf(`
        <tr>
            <td>%s</td>
			<td>%s</td>
            <td>-</td>
            <td class="tdspe">-</td>
        </tr>
    `, folderLink, fileInfo.Mode())
}

func createFileRow(file fs.DirEntry, currentPath string, fileInfo os.FileInfo) string {
	encodedFilePath := createEncodedPath(currentPath, file.Name())

	// Escapar nombres de archivos y rutas para su inserción segura en HTML
	escapedFileName := html.EscapeString(file.Name())
	escapedCurrentPath := html.EscapeString(currentPath)
	escapedencodedFilePath := html.EscapeString(encodedFilePath)

	downloadLink := fmt.Sprintf(`<a class="btn" href="/download/?path=%s"><i class="fa fa-download"></i></a>`, escapedencodedFilePath)
	deleteLink := fmt.Sprintf(`<a class="btn" href="/delete/?path=%s"><i class="fa fa-trash"></i></a>`, escapedencodedFilePath)
	copyURLButton := fmt.Sprintf(`<button class="btn" onclick="copyToClipboard('%s', '%s')"><i class="fa fa-link"></i></button>`, escapedCurrentPath, escapedFileName)
	fileSize, units := formatFileSize(fileInfo.Size())
	return fmt.Sprintf(`
        <tr>
            <td>%s</td>
			<td>%s</td>
            <td>%.2f %s</td>
            <td><div style="display: flex;">%s%s%s</div></td>
        </tr>
    `, escapedFileName, fileInfo.Mode(), fileSize, units, downloadLink, copyURLButton, deleteLink)
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
		return `<button class="btn" onclick="window.location.href='/'" style="height: 40px;width: 40px;"><i style="font-size: 20px;" class="fa fa-home"></i></button>`
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
