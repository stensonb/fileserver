package main

import (
	"log"
	"net/http"
)

func main() {
	fs := http.FileServer(http.Dir("./"))
	http.Handle("/", NewApacheLoggingHandler(fs, log.Writer()))
	log.Fatal(http.ListenAndServe(":1234", nil))
}
