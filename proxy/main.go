package main

import (
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
)

func main() {
	url, err := url.Parse("http://localhost:8081")

	if err != nil {
		log.Fatalf("error parsing service url: %v", err)
	}

	proxy := httputil.NewSingleHostReverseProxy(url)

	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, proxyErr error) {
		log.Printf("proxy encountered an error: %v", proxyErr)

		w.WriteHeader(http.StatusBadGateway)

		w.Write([]byte("the service is temporarily unavailable\n"))
	}

	mux := http.NewServeMux()
	mux.Handle("/", proxy)

	srv := &http.Server{
		Addr:    ":8080",
		Handler: mux,
	}

	log.Printf("proxy listening on :8080")
	log.Fatal(srv.ListenAndServe())
}
