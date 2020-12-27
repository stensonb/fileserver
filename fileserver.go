package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
)

var dataDir string
var uploadDir string
var listenPort int = 1234
var shutdownTimeout string = "60s"
var parsedShutdownTimeout time.Duration

func init() {
	var err error
	dataDir, err = os.Getwd()
	if err != nil {
		log.Fatal(err)
	}
	dataDir = filepath.Join(dataDir, "data")
	uploadDir = filepath.Join(dataDir, "uploads")

	flag.StringVar(&dataDir, "dataDir", dataDir, "directory to serve from")
	flag.StringVar(&uploadDir, "uploadDir", uploadDir, "directory to upload to")
	flag.IntVar(&listenPort, "p", listenPort, "port to listen on")
	flag.StringVar(&shutdownTimeout, "t", shutdownTimeout, "maximum time to wait for a clean shutdown")
}

func main() {
	flag.Parse()

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

	// the default http server
	srv := &http.Server{}

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

	r.Get("/*", wrapHandler(http.FileServer(http.Dir(dataDir))))
	r.Post("/uploadFile", uploadFile)

	listenAddr := fmt.Sprintf(":%s", strconv.Itoa(listenPort))
	log.Printf("Serving files from %s\n", dataDir)
	log.Printf("Uploaded files stored in %s\n", uploadDir)
	log.Printf("Listening on %s\n", listenAddr)

	// blocking call, running the server
	srv.Addr = listenAddr
	srv.Handler = r

	if err = srv.ListenAndServe(); err != http.ErrServerClosed {

		//	if err = srv.ListenAndServe(listenAddr, r); err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe: %v", err)
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

func wrapHandler(h http.Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		nfrw := &NotFoundRedirectRespWr{ResponseWriter: w}
		h.ServeHTTP(nfrw, r)
		if nfrw.status == 404 {
			log.Printf("Redirecting %s to index.html.", r.RequestURI)
			http.Redirect(w, r, "/index.html", http.StatusFound)
		}
	}
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

	resFile, err := os.Create(filepath.Join(uploadDir, handler.Filename))
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
