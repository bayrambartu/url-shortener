package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"os"
	"time"
	mongoo "url-shotener/mongo"

	_ "github.com/lib/pq"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type ShortenRequest struct {
	Address string `json:"address"`
}
type Server struct {
	DB      *sql.DB
	DBMongo *mongo.Client
}
type Link struct {
	ID          string    `json:"id"`
	ShortCode   string    `json:"short_code"`
	OriginalURL string    `json:"original_url"`
	CreatedAt   time.Time `json:"created_at"`
}
type ClickEvent struct {
	ID        string    `json:"id"`
	LinkID    string    `json:"link_id"`
	ClickedAt time.Time `json:"clicked_at"`
	UserAgent string    `json:"user_agent"`
	IPAddress string    `json:"ip_address"`
	Country   string    `json:"country"`
	Referrer  string    `json:"referrer"`
}

func main() {
	mux := http.NewServeMux()

	// DSN yerine DATABASE_URL okuyoruz
	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		log.Fatal("DATABASE_URL ortam değişkeni bulunamadı")
	}

	var db *sql.DB
	var err error

	// Docker'da veritabanının tamamen hazır olması 2-3 saniye sürebilir.
	// Hemen çökmek yerine kısa bir bekleme (retry) mekanizması ekliyoruz.
	for i := 0; i < 5; i++ {
		db, err = sql.Open("postgres", dbUrl)
		if err == nil {
			err = db.Ping()
			if err == nil {
				break // Bağlantı başarılı, döngüden çık
			}
		}
		fmt.Println("Veritabanı henüz hazır değil, tekrar deneniyor...")
		time.Sleep(2 * time.Second)
	}

	if err != nil {
		log.Fatal("Veritabanına bağlanılamadı:", err)
	}
	defer db.Close()

	fmt.Println("Veritabanına başarıyla bağlanıldı!")

	client := mongoo.ConnectMongoDB()
	defer client.Disconnect(context.Background())

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
		log.Fatal("Tablo oluşturulurken hata:", err)
	}

	server := &Server{DB: db, DBMongo: client}
	mux.HandleFunc("/shorten", server.AddLink)
	mux.HandleFunc("/code", server.GetOriginalURL)
	mux.HandleFunc("/getall", server.GetAllLinks)

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal("Error starting server:", err)
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

	host := r.RemoteAddr
	if ip, _, err := net.SplitHostPort(host); err == nil {
		host = ip
	}

	referrer := r.Referer()

	_, err = s.DBMongo.Database("shortener").Collection("ClickEvent").InsertOne(context.Background(), bson.M{
		"link_id":    link.ID,
		"user_agent": r.UserAgent(),
		"ip_address": host,
		"country":    "",
		"referrer":   referrer,
	})
	if err != nil {
		log.Println("click event insert error:", err)
	}

	count, _ := s.DBMongo.
		Database("shortener").
		Collection("ClickEvent").
		CountDocuments(context.Background(), bson.M{})

	log.Println("Document count:", count)

	if err != nil {
		log.Println("click event insert error:", err)
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
