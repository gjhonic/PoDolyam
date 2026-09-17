package main

import (
	"context"
	"log"
	"os"
	"podolyam/internal/storage"
	"podolyam/migrations"
	"time"
)

func main() {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("Задайте DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	db, err := storage.Open(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	if err = storage.Migrate(ctx, db, migrations.Files); err != nil {
		log.Fatal(err)
	}
	log.Print("Миграции применены")
}
