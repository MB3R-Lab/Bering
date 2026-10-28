package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	lookupURL := os.Getenv("LOOKUP_URL")
	if lookupURL == "" {
		lookupURL = "http://127.0.0.1:18082/lookup"
	}
	client := &http.Client{Timeout: 5 * time.Second}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	mux.HandleFunc("/call", func(w http.ResponseWriter, _ *http.Request) {
		response, err := client.Get(lookupURL)
		if err != nil {
			http.Error(w, "lookup unavailable", http.StatusBadGateway)
			return
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			http.Error(w, "lookup failed", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"result": "found"})
	})
	server := &http.Server{Addr: "127.0.0.1:18081", Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	log.Fatal(server.ListenAndServe())
}
