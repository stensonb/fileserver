package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"
)

const (
	DefaultShutdownTimeoutSeconds = 300
)

func main() {
	// server object
	var srv http.Server

	idleConnsClosed := make(chan struct{})

	// a go func to capture os.Interrupt and shutdown the server cleanly.
	// this times out (and force termination connections) after DefaultShutdownTimeoutSeconds
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt)
		<-sigint

		log.Println("Exiting nicely.  Interrupt again to force.")
		timeout_ctx, _ := context.WithTimeout(context.Background(), DefaultShutdownTimeoutSeconds*time.Second)
		if err := srv.Shutdown(timeout_ctx); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}

		close(idleConnsClosed)
	}()

	// server all files in the current directory
	fs := http.FileServer(http.Dir("./"))

	// log the requests in the apache format
	srv.Handler = NewApacheLoggingHandler(fs, log.Writer())

	// set the port to listen on
	srv.Addr = ":1234"

	// blocking call, running the server
	if err := srv.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatalf("HTTP server ListenAndServe: %v", err)
	}

	<-idleConnsClosed
}
