package main

import (
	"log"
	"net/http"
	"time"
)

func workHandler(w http.ResponseWriter, r *http.Request) {
	time.Sleep(200 * time.Millisecond)

	w.Write([]byte("work done\n"))
	log.Printf("work request completed")
}

func main() {
	mux := http.NewServeMux()
	mux.HandleFunc("/work", workHandler)

	srv := &http.Server{
		Addr:    ":8081",
		Handler: mux,
	}

	log.Printf("listening on :8081")
	log.Fatal(srv.ListenAndServe())
}
