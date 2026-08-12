package postgres

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

func ConnectPostgres(dbURL string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, err
	}

	for i := 0; i < 5; i++ {
		if err := db.Ping(); err == nil {
			return db, nil
		}

		fmt.Println("PostgreSQL is not ready yet, retrying...")
		time.Sleep(2 * time.Second)
	}

	db.Close()

	return nil, fmt.Errorf("PostgreSQL's not ready after multiple attempts")
}
