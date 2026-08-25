package main

import (
	"encoding/json"
	"log"
	"os"
	"time"
	"url-shotener/internal/models"
	"url-shotener/internal/rabbitmq"
	"url-shotener/postgres"

	_ "github.com/lib/pq"
)

func main() {

	dbUrl := os.Getenv("DATABASE_URL")
	if dbUrl == "" {
		log.Fatal("DATABASE_URL ortam değişkeni bulunamadı")
	}

	db, err := postgres.ConnectPostgres(dbUrl)
	if err != nil {
		log.Fatal("PostgreSQL'a bağlanılamadı:", err)
	}
	defer db.Close()

	_, rabbitMQChan, err := rabbitmq.Connect(os.Getenv("RABBITMQ_URI"))
	if err != nil {
		log.Fatal(err)
	}
	defer rabbitMQChan.Close()

	msgs, err := rabbitMQChan.Consume(
		rabbitmq.QueueName,
		"",
		false, // manual acknowledgment
		false,
		false,
		false,
		nil,
	)

	if err != nil {
		log.Println("Failed to consume messages:", err)
		return
	}

	for msg := range msgs {
		log.Printf("Received message: %s", msg.Body)

		var linkMsg models.LinkMessage
		if err := json.Unmarshal(msg.Body, &linkMsg); err != nil {
			log.Println("Failed to parse message:", err)
			msg.Nack(false, false)
			continue
		}

		log.Println("Processing link ID:", linkMsg.ID)
		time.Sleep(10 * time.Second) // Simulate processing time

		_, err = db.Exec("UPDATE links SET is_verified = true WHERE id = $1", linkMsg.ID)
		if err != nil {
			log.Println("Failed to update link verification:", err)
			continue
		}

		if err := msg.Ack(false); err != nil {
			log.Println("ACK failed:", err)
			continue
		}
		log.Println("ack sent")
	}
}
