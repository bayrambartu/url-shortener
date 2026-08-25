package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"net/http"
	"os"
	"url-shotener/internal/handlers"
	"url-shotener/internal/rabbitmq"
	mongoo "url-shotener/mongo"
	"url-shotener/postgres"

	_ "github.com/lib/pq"
)

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
        created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		is_verified BOOLEAN DEFAULT FALSE
    );
    `
	_, err = db.Exec(createTableQuery)
	if err != nil {
		log.Fatal("Tablo oluşturulurken hata:", err)
	}
	rabbitMQConn, rabbitMQChan, err := rabbitmq.Connect(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitMQConn.Close()
	defer rabbitMQChan.Close()

	server := &handlers.Server{DB: db, DBMongo: client, RabbitMQConn: rabbitMQConn, RabbitMQChan: rabbitMQChan}

	mux.HandleFunc("/shorten", server.AddLink)
	mux.HandleFunc("/code", server.GetOriginalURL)
	mux.HandleFunc("/getall", server.GetAllLinks)

	fmt.Println("Server is running on http://localhost:8080")
	if err := http.ListenAndServe(":8080", mux); err != nil {
		log.Fatal("Error starting server:", err)
	}
}
