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
	"sync"
	"time"
	mongoo "url-shotener/mongo"
	"url-shotener/postgres"

	_ "github.com/lib/pq"
	ampq "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type ShortenRequest struct {
	Address string `json:"address"`
}
type Server struct {
	DB           *sql.DB
	DBMongo      *mongo.Client
	rabbitMQConn *ampq.Connection
	rabbitMQChan *ampq.Channel
	rabbitMu     sync.Mutex
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

	db, err = postgres.ConnectPostgres(dbUrl)
	if err != nil {
		log.Fatal("PostgreSQL'a bağlanılamadı:", err)
	}
	defer db.Close()

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
	// RabbitMQ connection
	rabbitMQConn, err := ampq.Dial(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Println("Failed to connect to RabbitMQ:", err)
		return
	}
	fmt.Println("Connected to RabbitMQ")
	defer rabbitMQConn.Close()

	// RabbitMQ channel
	rabbitCh, err := rabbitMQConn.Channel()
	if err != nil {
		log.Fatal("Failed to open RabbitMQ channel:", err)
	}
	fmt.Println("RabbitMQ channel opened")
	defer rabbitCh.Close()

	err = rabbitCh.ExchangeDeclare("logs", "direct", true, false, false, false, nil)
	if err != nil {
		log.Fatal("Failed to declare exchange:", err)
	}
	fmt.Println("Exchange declared")

	_, err = rabbitCh.QueueDeclare("shortened_urls", true, false, false, false, nil)
	if err != nil {
		log.Fatal("Failed to declare queue:", err)
	}
	fmt.Println("Queue declared")

	err = rabbitCh.QueueBind("shortened_urls", "shortened_urls_routing_key", "logs", false, nil)
	if err != nil {
		log.Fatal("Failed to bind queue:", err)
	}
	fmt.Println("Queue bound to exchange")

	server := &Server{DB: db, DBMongo: client, rabbitMQConn: rabbitMQConn, rabbitMQChan: rabbitCh}

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
	log.Println("===== AddLink called =====")
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
	var count int
	s.DB.QueryRow("SELECT COUNT(*) FROM links WHERE original_url = $1", req.Address).Scan(&count)
	if count > 0 {
		http.Error(w, "Address already exists", http.StatusConflict)
		return
	}

	_, err = s.DB.Exec("INSERT INTO links (short_code, original_url) VALUES ($1, $2)", generatedString, req.Address)
	if err != nil {
		http.Error(w, "Failed to store URL", http.StatusInternalServerError)
		return
	}
	var id int
	row := s.DB.QueryRow("SELECT id FROM links WHERE short_code = $1", generatedString).Scan(&id)
	if row != nil {
		http.Error(w, "Failed to retrieve link ID", http.StatusInternalServerError)
		return
	}

	type LinkMessage struct {
		ID      int    `json:"id"`
		Address string `json:"address"`
	}

	msg := LinkMessage{ID: id, Address: req.Address}
	body, _ := json.Marshal(msg)

	s.rabbitMu.Lock()
	defer s.rabbitMu.Unlock()

	err = s.rabbitMQChan.Publish(
		"logs",
		"shortened_urls_routing_key",
		false,
		false,
		ampq.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
	if err != nil {
		log.Println("Failed to publish message:", err)
		http.Error(w, "Failed to queue link for verification", http.StatusInternalServerError)
		return
	}
	fmt.Println("Message published to RabbitMQ:", string(body))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"shortened_url is created": generatedString})

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
