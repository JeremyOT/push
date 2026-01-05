package main

import (
	"database/sql"
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

//go:embed static/*
var staticFS embed.FS

func main() {
	address := flag.String("address", "127.0.0.1", "BIND_ADDRESS")
	port := flag.Int("port", 8089, "PORT")
	dbPath := flag.String("database", "./push.sqlite", "DATABASE")
	flag.Parse()

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticRoot)))
	
	// Placeholder handlers
	http.HandleFunc("/interactions", handleInteractions(db))

	addr := fmt.Sprintf("%s:%d", *address, *port)
	log.Printf("Server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func initDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS interactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);`
	_, err := db.Exec(query)
	return err
}

func handleInteractions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			// Handle posting new interaction
			fmt.Fprintf(w, "Post interaction")
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}
