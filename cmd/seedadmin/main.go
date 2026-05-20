// seedadmin: create or update an admin user.
//
//	seedadmin <email> <password>
//
// Uses DATABASE_URL from the environment.
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"mediaproxy/internal/auth"
	"mediaproxy/internal/db"

	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()

	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: seedadmin <email> <password>")
		os.Exit(2)
	}
	email := strings.ToLower(strings.TrimSpace(os.Args[1]))
	pass := os.Args[2]
	if len(pass) < 8 {
		log.Fatal("password must be at least 8 characters")
	}

	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		log.Fatal("DATABASE_URL is required")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pg, err := db.NewPostgres(ctx, dbURL)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer pg.Close()

	hash, err := auth.HashPassword(pass)
	if err != nil {
		log.Fatalf("hash: %v", err)
	}

	var id int64
	err = pg.QueryRow(ctx, `
		INSERT INTO admin_users (email, password_hash, role, status)
		VALUES ($1, $2, 'admin', 'active')
		ON CONFLICT (email) DO UPDATE SET password_hash = EXCLUDED.password_hash, status = 'active'
		RETURNING id
	`, email, hash).Scan(&id)
	if err != nil {
		log.Fatalf("upsert: %v", err)
	}
	fmt.Printf("admin user id=%d email=%s ready\n", id, email)
}
