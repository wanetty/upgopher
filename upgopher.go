	package main

	import (
		"encoding/base64"
		"flag"
		"fmt"
		"io"
		"net/http"
		"os"
		"path/filepath"
		"io/ioutil"
		"crypto/subtle"
		"log"
		"strings"

		"upgopher/internal/statics"
	)

	func main() {
		port := flag.Int("port", 9090, "port number")
		dir := flag.String("dir", "./uploads", "directory path")
		user := flag.String("user", "", "username for authentication")
		pass := flag.String("pass", "", "password for authentication")
		useTLS := flag.Bool("tls", false, "utilizar HTTPS")
		certFile := flag.String("cert", "", "certificado para HTTPS")
		keyFile := flag.String("key", "", "clave privada para HTTPS")
		flag.Parse()

		if _, err := os.Stat(*dir); os.IsNotExist(err) {
			os.MkdirAll(*dir, 0755)
		}

		fileHandlerWithDir := func(w http.ResponseWriter, r *http.Request) {
			fileHandler(w, r, *dir)
		}

		if *useTLS && (*certFile == "" || *keyFile == "") {
			log.Fatalf("Debe proporcionar el certificado y la clave privada para usar TLS")
		}

		if (*user != "" && *pass == "") || (*user == "" && *pass != "") {
			log.Fatalf("If you use the username or password you have to use both.")
			return
		}

		uploadHandler := func(w http.ResponseWriter, r *http.Request) {
			log.Printf("[%s] %s%s %s\n", r.Method, "/uploads/", r.URL.String(), r.RemoteAddr)

			encodedFilePath := r.URL.Query().Get("path")

			decodedFilePath, err := base64.StdEncoding.DecodeString(encodedFilePath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			fullFilePath := filepath.Join(*dir, string(decodedFilePath))

			if isUnsafePath(fullFilePath) {
				http.Error(w, "Bad path", http.StatusNotFound)
				return
			}

			if _, err := os.Stat(fullFilePath); os.IsNotExist(err) {
				http.Error(w, "File not found", http.StatusNotFound)
				return
			}

			http.ServeFile(w, r, fullFilePath)
		}

		if *user != "" && *pass != "" {
			userByte := []byte(*user)
			passByte := []byte(*pass)
			http.HandleFunc("/", basicAuth(fileHandlerWithDir, userByte, passByte))
			http.Handle("/uploads/", http.StripPrefix("/uploads/", basicAuth(http.HandlerFunc(uploadHandler), userByte, passByte)))
		} else {
			http.HandleFunc("/", fileHandlerWithDir)
			http.Handle("/uploads/", http.StripPrefix("/uploads/", http.HandlerFunc(uploadHandler)))
		}

		addr := fmt.Sprintf(":%d", *port)
		log.Printf("Web server on %s", addr)

		if *useTLS {
			log.Printf("Usando TLS")
			if err := http.ListenAndServeTLS(addr, *certFile, *keyFile, nil); err != nil {
				log.Fatalf("Error al iniciar el servidor HTTPS: %v", err)
			}
		} else {
			log.Printf("Usando HTTP")
			if err := http.ListenAndServe(addr, nil); err != nil {
				log.Fatalf("Error al iniciar el servidor HTTP: %v", err)
			}
		}
	}

	func isUnsafePath(inputPath string) bool {
		return strings.Contains(inputPath, "../") || strings.Contains(inputPath, "..")
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
		log.Printf("[%s] %s %s\n", r.Method, r.URL.String(), r.RemoteAddr)
		currentPath := r.URL.Query().Get("path")
		
		if currentPath != "" {
			decodedPath, err := base64.StdEncoding.DecodeString(currentPath)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			dir = filepath.Join(dir, string(decodedPath))
			if isUnsafePath(dir) {
				http.Error(w, "Bad path", http.StatusNotFound)
				return
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
		files, err := ioutil.ReadDir(dir)
		if err != nil {
			http.Error(w, "The path not exists", http.StatusInternalServerError)
			return
		}
	
		fileNames := make([]string, 0, len(files))
		for _, file := range files {
			if !file.IsDir() {
				fileNames = append(fileNames, file.Name())
			}
		}
	
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		table, err := createTable(files, dir, currentPath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		backButton := createBackButton(currentPath)
		fmt.Fprintf(w, statics.GetTemplates(table, backButton))
	}
	
	func createTable(files []os.FileInfo, dir string, currentPath string) (string, error) {
		table := ""
		for _, file := range files {
			if file.Name()[0] == '.' {
				continue
			}
	
			fileName := file.Name()
			filePath := filepath.Join(dir, fileName)
			fileInfo, err := os.Stat(filePath)
			if err != nil {
				return "", err
			}
	
			if file.IsDir() {
				table += createFolderRow(file, currentPath)
			} else {
				table += createFileRow(file, currentPath, fileInfo)
			}
		}
		return table, nil
	}
	
	func createFolderRow(file os.FileInfo, currentPath string) string {
		encodedPath := createEncodedPath(currentPath, file.Name())
		folderLink := fmt.Sprintf(`<a href="/?path=%s">%s</a>`, encodedPath, file.Name())
		return fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>-</td>
				<td class="tdspe">-</td>
			</tr>
		`, folderLink)
	}
	
	func createFileRow(file os.FileInfo, currentPath string, fileInfo os.FileInfo) string {
		encodedFilePath := createEncodedPath(currentPath, file.Name())
		downloadLink := fmt.Sprintf(`<a class="btn" href="/uploads/?path=%s"><i class="fa fa-download"></i></a>`, encodedFilePath)
		copyURLButton := fmt.Sprintf(`<button class="btn" onclick="copyToClipboard('%s')"><i class="fa fa-link"></i></button>`, encodedFilePath)
		fileSize, units := formatFileSize(fileInfo.Size())
		return fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%.2f %s</td>
				<td><div style="display: flex;">%s%s</div></td>
			</tr>
		`, file.Name(), fileSize, units, downloadLink, copyURLButton)
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
			return `<button class="btn" onclick="window.location.href='/'" style="height: 50px;width: 50px;"><i class="fa fa-home"></i></button>`
		}
		return ""
	}