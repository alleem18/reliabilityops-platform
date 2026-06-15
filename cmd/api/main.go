package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

type jsonResponse map[string]string

func writeJSON(w http.ResponseWriter, status int, data jsonResponse) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	err := json.NewEncoder(w).Encode(data)
	if err != nil {
		log.Println("Failed to write JSON response:", err)
	}
}
func healthHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jsonResponse{
		"status": "ok",
	})
}

func rootHandler(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, jsonResponse{
		"message": "ReliabilityOps API is running",
		"status":  "ok",
	})
}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/", rootHandler)
	mux.HandleFunc("/health", healthHandler)

	port := "8080"
	addr := ":" + port

	fmt.Println("Starting ReliabilityOps API on port", port)

	err := http.ListenAndServe(addr, mux)
	if err != nil {
		log.Fatal(err)
	}
}
