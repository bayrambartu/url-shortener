package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"math/rand/v2"
	"net"
	"net/http"
	"sync"

	"url-shotener/internal/models"

	ampq "github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

type Server struct {
	DB           *sql.DB
	DBMongo      *mongo.Client
	RabbitMQConn *ampq.Connection
	RabbitMQChan *ampq.Channel
	RabbitMu     sync.Mutex
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

	var req models.ShortenRequest
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

	msg := models.LinkMessage{ID: id, Address: req.Address}
	body, _ := json.Marshal(msg)

	s.RabbitMu.Lock()
	defer s.RabbitMu.Unlock()

	err = s.RabbitMQChan.Publish(
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
	var link models.Link
	err := s.DB.QueryRow("SELECT id, short_code, original_url, created_at FROM links WHERE short_code = $1", shortenedURL).Scan(&link.ID, &link.ShortCode, &link.OriginalURL, &link.CreatedAt)

	if err != nil {
		if err == sql.ErrNoRows {
			// http.Error(w, "Shortened URL not found", http.StatusNotFound)
			http.Redirect(w, r, "/", http.StatusFound)
			return
		}
		log.Println("GetOriginalURL DB error:", err)
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
	rows, err := s.DB.Query("SELECT id, short_code, original_url, created_at FROM links")
	if err != nil {
		http.Error(w, "Error fetching URLs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var links []models.Link
	for rows.Next() {
		var link models.Link
		err := rows.Scan(&link.ID, &link.ShortCode, &link.OriginalURL, &link.CreatedAt)
		if err != nil {
			log.Println("GetAllLinks scan error:", err)
			http.Error(w, "Error scanning URL", http.StatusInternalServerError)
			return
		}
		links = append(links, link)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(links)
}
