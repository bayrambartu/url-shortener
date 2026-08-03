package main

import (
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type ShortenRequest struct {
	Address string `json:"address"`
}

var urlStore = map[string]string{}

func main() {
	mux := http.NewServeMux()

	mux.HandleFunc("/shorten", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		generatedString := generateRandomString(10)

		var req ShortenRequest
		err := json.NewDecoder(r.Body).Decode(&req)
		if err != nil {
			http.Error(w, "Invalid request body", http.StatusBadRequest)
			return
		}
		urlStore[generatedString] = req.Address
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"shortened_url": generatedString})

	})

	mux.HandleFunc("/code", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		shortenedURL := r.URL.Query().Get("shortened_url")
		if shortenedURL == "" {
			http.Error(w, "Missing shortened_url parameter", http.StatusBadRequest)
			return
		}
		originalURL, exists := urlStore[shortenedURL]
		if !exists {
			// http.Error(w, "Shortened URL not found", http.StatusNotFound)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		//	json.NewEncoder(w).Encode(map[string]string{"original_url": originalURL})
		http.Redirect(w, r, originalURL, http.StatusFound)
	})
	fmt.Println(urlStore)
	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		fmt.Println("Error starting server:", err)
	}

}

func generateRandomString(length int) string {
	s := make([]byte, length)
	for i := range s {
		s[i] = charset[rand.IntN(len(charset))]
	}

	return string(s)

}
