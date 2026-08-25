package rabbitmq

import (
	"fmt"
	"time"

	ampq "github.com/rabbitmq/amqp091-go"
)

const (
	ExchangeName = "logs"
	QueueName    = "shortened_urls"
	RoutingKey   = "shortened_urls_routing_key"
)

// retry
func Connect(uri string) (*ampq.Connection, *ampq.Channel, error) {
	var conn *ampq.Connection
	var err error

	for i := 0; i < 5; i++ {
		conn, err = ampq.Dial(uri)
		if err == nil {
			break
		}
		fmt.Println("failed to connect to RabbitMQ, retrying in 2 seconds...")
		time.Sleep(2 * time.Second)
	}
	if err != nil {
		return nil, nil, fmt.Errorf("failed to connect to RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open a channel: %w", err)
	}

	err = ch.ExchangeDeclare(ExchangeName, "direct", true, false, false, false, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to declare exchange: %w", err)
	}

	_, err = ch.QueueDeclare(QueueName, true, false, false, false, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to declare queue: %w", err)
	}

	err = ch.QueueBind(QueueName, RoutingKey, ExchangeName, false, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to bind queue: %w", err)
	}

	return conn, ch, nil
}
