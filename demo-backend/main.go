package main

import (
	"encoding/json"
	"log"
	"net/http"
	"os"
	"strconv"
	"sync/atomic"
)

var activeConnections int64

func main() {
	name := os.Getenv("BACKEND_NAME")
	if name == "" {
		name = "demo"
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "9001"
	}

	http.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":                "ok",
			"backend":               name,
			"active_connections":    atomic.LoadInt64(&activeConnections),
			"available_connections": 100,
		})
	})

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&activeConnections, 1)
		defer atomic.AddInt64(&activeConnections, -1)

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{
			"backend": name,
			"method":  r.Method,
			"path":    r.URL.Path,
		})
	})

	log.Printf("Demo backend %s listening on :%s", name, port)
	log.Fatal(http.ListenAndServe(":"+strconv.Itoa(parsePort(port)), nil))
}

func parsePort(value string) int {
	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		log.Fatalf("invalid PORT %q", value)
	}
	return port
}
