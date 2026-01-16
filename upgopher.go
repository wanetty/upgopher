package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"flag"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/wanetty/upgopher/internal/server"
)

//go:embed static/favicon.ico
var favicon embed.FS

//go:embed static/logopher.webp
var logo embed.FS

// global vars
var quite bool = false
var version = "1.13.0"
var showHiddenFiles bool = false
var disableHiddenFiles bool = false
var readOnly bool = false
var sharedClipboard string = ""
var clipboardMutex sync.Mutex

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

func isFlagPassed(name string) bool {
	found := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}
