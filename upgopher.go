package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"io/ioutil"
	"crypto/subtle"
	"log"

	"upgopher/internal/statics"
)

func main() {

	port := flag.Int("port", 9090, "port number")
	dir := flag.String("dir", "./uploads", "directory path")
	user := flag.String("user", "", "username for authentication")
	pass  := flag.String("pass", "", "password for authentication")
	useTLS := flag.Bool("tls", false, "utilizar HTTPS")
	certFile := flag.String("cert", "", "certificado para HTTPS")
	keyFile := flag.String("key", "", "clave privada para HTTPS")
	flag.Parse()

	// Ensure the directory path exists.
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
	
	if *user != "" && *pass != "" {
		userByte := []byte(*user)
		passByte := []byte(*pass)
		http.HandleFunc("/", basicAuth(fileHandlerWithDir, userByte, passByte))
		fs := http.FileServer(http.Dir(*dir))
		protectedHandler := basicAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Serve the file using the file server handler
			fs.ServeHTTP(w, r)
		}), userByte, passByte)
		
		// Serve the files under the "/uploads" path
		http.Handle("/uploads/", http.StripPrefix("/uploads/", protectedHandler))

	}else{
		http.HandleFunc("/", fileHandlerWithDir)
		fs := http.FileServer(http.Dir(*dir))
		http.Handle("/uploads/", http.StripPrefix("/uploads/", fs))
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
	log.Printf("[%s] %s %s\n", r.Method, r.URL.Path, r.RemoteAddr)
	if r.Method == http.MethodPost {
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
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}

	// Serve the file list page for GET requests
	files, err := ioutil.ReadDir(dir)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	fileNames := make([]string, 0, len(files))
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		fileNames = append(fileNames, file.Name())
	}
	

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	var table string
	for _, file := range files {
		if file.Name()[0] == '.' {
			continue // Ignore hidden files and folders
		}

		fileName := file.Name()
		filePath := filepath.Join(dir, fileName)
		// Get file size in bytes
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if file.IsDir() {
			fileSize := "-"
			// Add row to the table
			table += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%s</td>
				<td class="tdspe">-</td>
			</tr>
		`, fileName, fileSize)
		}else {
			var fileSize float64
			var units string
			downloadLink := fmt.Sprintf(`<a class="btn" href="/uploads/%s"><i class="fa fa-download"></i></a>`, fileName)
			copyURLButton := fmt.Sprintf(`<button class="btn" onclick="copyToClipboard('%s')"><i class="fa fa-link"></i></button>`, fileName)
			if fileInfo.Size() < 1000 {
				fileSize = float64(fileInfo.Size())
				units = "bytes"
			} else if  fileInfo.Size() < 1000000 {
				fileSize = float64(fileInfo.Size()) / 1000
				units = "KBytes"

			} else {
				fileSize = float64(fileInfo.Size()) / 1000000
				units = "MBytes"
			}
			
			// Create download link and copy URL button
			
			// Add row to the table
			table += fmt.Sprintf(`
			<tr>
				<td>%s</td>
				<td>%.2f %s</td>
				<td>%s%s</td>
			</tr>
		`,fileName, fileSize,units,downloadLink, copyURLButton)
		}
		
	}
	
	// End the HTML code
	fmt.Fprintf(w,statics.GetTemplates(table))
}	
