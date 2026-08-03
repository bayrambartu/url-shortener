package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"net/http"
	"time"

	_ "github.com/lib/pq"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type ShortenRequest struct {
	Address string `json:"address"`
}
type Server struct {
	DB *sql.DB
}
type Link struct {
	ID          string    `json:"id"`
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	CreatedAt   time.Time `json:"created_at"`
}

func main() {
	mux := http.NewServeMux()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=shortener sslmode=disable"

	var err error

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("Error opening database:", err)
		return
	}
	createTableQuery := `
	CREATE TABLE IF NOT EXISTS links (
		id SERIAL PRIMARY KEY,
		short_code VARCHAR(255) UNIQUE NOT NULL,
		original_url TEXT NOT NULL,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	`
	_, err = db.Exec(createTableQuery)
	if err != nil {
		fmt.Println("Error creating table:", err)
		return
	}
	defer db.Close()

	server := &Server{DB: db}
	mux.HandleFunc("/shorten", server.AddLink)
	mux.HandleFunc("/code", server.GetOriginalURL)
	mux.HandleFunc("/getall", server.GetAllLinks)

	fmt.Println("Connecting to database...")
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

func (s *Server) AddLink(w http.ResponseWriter, r *http.Request) {
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
	_, err = s.DB.Exec("INSERT INTO links (short_code, original_url) VALUES ($1, $2)", generatedString, req.Address)
	if err != nil {
		http.Error(w, "Failed to store URL", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"shortened_url": generatedString})

}

func (s *Server) GetOriginalURL(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	shortenedURL := r.URL.Query().Get("shortened_url")
	if shortenedURL == "" {
		http.Error(w, "Missing shortened_url parameter", http.StatusBadRequest)
		return
	}
	var link Link
	err := s.DB.QueryRow("SELECT * FROM links WHERE short_code = $1", shortenedURL).Scan(&link.ID, &link.ShortCode, &link.OriginalURL, &link.CreatedAt)
	if err != nil {
		if err == sql.ErrNoRows {
			// http.Error(w, "Shortened URL not found", http.StatusNotFound)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		http.Error(w, "Error fetching URL", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	//json.NewEncoder(w).Encode(link)
	http.Redirect(w, r, link.OriginalURL, http.StatusFound)
}

func (s *Server) GetAllLinks(w http.ResponseWriter, r *http.Request) {

	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	rows, err := s.DB.Query("SELECT * FROM links")
	if err != nil {
		http.Error(w, "Error fetching URLs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var links []Link
	for rows.Next() {
		var link Link
		err := rows.Scan(&link.ID, &link.ShortCode, &link.OriginalURL, &link.CreatedAt)
		if err != nil {
			http.Error(w, "Error scanning URL", http.StatusInternalServerError)
			return
		}
		links = append(links, link)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}
