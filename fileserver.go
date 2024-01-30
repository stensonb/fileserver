package main

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"embed"
	"encoding/pem"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	qrcode "github.com/skip2/go-qrcode"
)

const (
	domain = "siliconvortex.com"
	name   = "fileserver"
)

var dataDir string
var uploadDir string
var listenAddress string = getLocalIP()
var listenPort int = 1234
var printQRCode bool = true
var shutdownTimeout string = "60s"
var parsedShutdownTimeout time.Duration
var tlsEnabled bool = true
var tlsSelfSigned bool = true
var tlsCertPath string = "cert.pem"
var tlsKeyPath string = "cert.key"

//go:embed html/*
var content embed.FS

func init() {
	var err error
	baseDir, err := os.Getwd()
	if err != nil {
		log.Fatal(err)
	}

	tlsCertPath = filepath.Join(baseDir, tlsCertPath)
	tlsKeyPath = filepath.Join(baseDir, tlsKeyPath)

	dataDir = filepath.Join(baseDir, "data")
	uploadDir = filepath.Join(dataDir, "uploads")

	flag.StringVar(&dataDir, "dataDir", dataDir, "directory to serve from")
	flag.StringVar(&uploadDir, "uploadDir", uploadDir, "directory to upload to")
	flag.StringVar(&listenAddress, "address", listenAddress, "address to listen on")
	flag.IntVar(&listenPort, "port", listenPort, "port to listen on")
	flag.BoolVar(&printQRCode, "qrcode", printQRCode, "print QRCode")
	flag.BoolVar(&tlsEnabled, "tls", tlsEnabled, "host with tls")
	flag.BoolVar(&tlsSelfSigned, "tls-self-signed", tlsSelfSigned, "use self-signed cert/key combo")
	flag.StringVar(&tlsCertPath, "tls-cert-path", tlsCertPath, "path for tls cert if tls-self-signed=false")
	flag.StringVar(&tlsKeyPath, "tls-key-path", tlsKeyPath, "path for tls cert if tls-self-signed=false")
	flag.StringVar(&shutdownTimeout, "timeout", shutdownTimeout, "maximum time to wait for a clean shutdown")
}

func rsaPrivateKeyAsPemBytes(privkey *rsa.PrivateKey) []byte {
	privkey_bytes := x509.MarshalPKCS1PrivateKey(privkey)
	privkey_pem := pem.EncodeToMemory(
		&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: privkey_bytes,
		},
	)
	return privkey_pem
}

func tlsConfigSelfSigned() (*tls.Config, error) {
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber: big.NewInt(now.Unix()),
		Subject: pkix.Name{
			CommonName:         fmt.Sprintf("%s.%s", name, domain),
			Country:            []string{"Earth"},
			Organization:       []string{"siliconvortex.com"},
			OrganizationalUnit: []string{name},
		},
		NotBefore:             now,
		NotAfter:              now.AddDate(0, 0, 7), // Valid for seven days
		SubjectKeyId:          []byte(name),
		BasicConstraintsValid: true,
		IsCA:                  true,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		KeyUsage: x509.KeyUsageKeyEncipherment |
			x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
	}

	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to genrate key: %w", err)
	}

	certBytes, err := x509.CreateCertificate(rand.Reader, template, template, privKey.Public(), privKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	return &tls.Config{
		ServerName: name,
		Certificates: []tls.Certificate{
			{
				Certificate: [][]byte{certBytes},
				PrivateKey:  privKey,
			},
		},
	}, nil
}

func main() {
	flag.Parse()

	dataDir = filepath.Clean(dataDir)
	uploadDir = filepath.Clean(uploadDir)

	var err error
	err = os.MkdirAll(dataDir, 0700)
	if err != nil {
		log.Println(err)
	}
	err = os.MkdirAll(uploadDir, 0700)
	if err != nil {
		log.Println(err)
	}

	parsedShutdownTimeout, err := time.ParseDuration(shutdownTimeout)
	if err != nil {
		log.Fatal(err)
	}

	ipPortCombo := fmt.Sprintf("%s:%s", listenAddress, strconv.Itoa(listenPort))

	scheme := "http"
	srv := &http.Server{}

	if tlsEnabled {
		scheme = "https"

		var tlsConfig *tls.Config
		if tlsSelfSigned {
			// the default http server with tlsConfig
			tlsConfig, err = tlsConfigSelfSigned()
			if err != nil {
				log.Fatal(err)
			}
		}

		srv.TLSConfig = tlsConfig
	}

	theURL := url.URL{
		Scheme: scheme,
		Host:   ipPortCombo,
	}

	idleConnsClosed := make(chan struct{})

	// a go func to capture os.Interrupt and shutdown the server cleanly.
	// this times out (and force termination connections) after parsedShutdownTimeout
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Println("Exiting nicely.  Interrupt again to force.")
		timeoutCtx, cancel := context.WithTimeout(context.Background(), parsedShutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(timeoutCtx); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(idleConnsClosed)
	}()

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)

	fsys, err := fs.Sub(content, "html")
	if err != nil {
		log.Fatal(err)
	}

	FileServer(r, "/", http.FS(fsys))
	FileServer(r, "/data", http.Dir(dataDir))
	FileServer(r, "/uploads", http.Dir(uploadDir))
	r.Post("/uploadFile", uploadFile)

	log.Printf("Serving files from %s\n", dataDir)
	log.Printf("Uploaded files stored in %s\n", uploadDir)
	log.Printf("Listening at %s\n", theURL.String())
	if printQRCode {
		log.Printf("\n%s", getQRCode(theURL.String()))
	}

	// blocking call, running the server
	srv.Addr = theURL.Host
	srv.Handler = r

	if tlsEnabled {
		if tlsSelfSigned {
			// server already as tlsConfig, so it will ignore the cert/key empty strings here
			if err = srv.ListenAndServeTLS("", ""); err != http.ErrServerClosed {
				log.Fatalf("HTTP server ListenAndServeTLS self-signed: %v", err)
			}
		} else {
			if err = srv.ListenAndServeTLS(tlsCertPath, tlsKeyPath); err != http.ErrServerClosed {
				log.Fatalf("HTTP server ListenAndServeTLS path: %v", err)
			}
		}
	} else {
		if err = srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("HTTP server ListenAndServe: %v", err)
		}
	}

	<-idleConnsClosed
	log.Println("Done.")
}

type NotFoundRedirectRespWr struct {
	http.ResponseWriter // We embed http.ResponseWriter
	status              int
}

func (w *NotFoundRedirectRespWr) WriteHeader(status int) {
	w.status = status // Store the status for our own use
	if status != http.StatusNotFound {
		w.ResponseWriter.WriteHeader(status)
	}
}

func (w *NotFoundRedirectRespWr) Write(p []byte) (int, error) {
	if w.status != http.StatusNotFound {
		return w.ResponseWriter.Write(p)
	}
	return len(p), nil // Lie that we successfully written it
}

func uploadFile(w http.ResponseWriter, r *http.Request) {
	//upload size
	err := r.ParseMultipartForm(200000) // grab the multipart form
	if err != nil {
		fmt.Fprintln(w, err)
	}

	//reading original file
	file, handler, err := r.FormFile("originalFile")
	if err != nil {
		log.Println("Error Retrieving the File")
		log.Println(err)
		return
	}
	defer file.Close()

	fileToCreate := filepath.Clean(filepath.Join(uploadDir, handler.Filename))

	resFile, err := os.Create(fileToCreate)
	if err != nil {
		fmt.Fprintln(w, err)
	}
	defer resFile.Close()

	if err == nil {
		io.Copy(resFile, file)
		defer resFile.Close()
		fmt.Fprintf(w, "Successfully Uploaded Original File\n")
	}
}

// getLocalIP returns the non loopback local IP of the host
func getLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		// check the address type and if it is not a loopback the display it
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

func getQRCode(s string) string {
	q, err := qrcode.New(s, qrcode.Low)
	if err != nil {
		log.Fatal(err)
	}

	return q.ToString(false)
}

// FileServer conveniently sets up a http.FileServer handler to serve
// static files from a http.FileSystem.
func FileServer(r chi.Router, path string, root http.FileSystem) {
	if strings.ContainsAny(path, "{}*") {
		panic("FileServer does not permit any URL parameters.")
	}

	if path != "/" && path[len(path)-1] != '/' {
		r.Get(path, http.RedirectHandler(path+"/", http.StatusMovedPermanently).ServeHTTP)
		path += "/"
	}
	path += "*"

	r.Get(path, func(w http.ResponseWriter, r *http.Request) {
		rctx := chi.RouteContext(r.Context())
		pathPrefix := strings.TrimSuffix(rctx.RoutePattern(), "/*")
		fs := http.StripPrefix(pathPrefix, http.FileServer(root))
		fs.ServeHTTP(w, r)
	})
}
