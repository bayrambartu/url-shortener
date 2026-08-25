package models

import "time"

type ShortenRequest struct {
	Address string `json:"address"`
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

type LinkMessage struct {
	ID      int    `json:"id"`
	Address string `json:"address"`
}
