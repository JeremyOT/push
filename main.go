package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed static/*
var staticFS embed.FS

type Interaction struct {
	ID        int64     `json:"id"`
	Message   string    `json:"message"`
	Timestamp time.Time `json:"timestamp"`
}

var vapidPrivateKey string
var vapidPublicKey string
var serverHostname string

func main() {
	defaultHostname, _ := os.Hostname()
	address := flag.String("address", "127.0.0.1", "BIND_ADDRESS")
	port := flag.Int("port", 8089, "PORT")
	dbPath := flag.String("database", "./push.sqlite", "DATABASE")
	hostname := flag.String("hostname", defaultHostname, "HOSTNAME for push notifications")
	flag.Parse()

	serverHostname = *hostname

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if err := initDB(db); err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}

	if err := initVAPID(db); err != nil {
		log.Fatalf("Failed to init VAPID keys: %v", err)
	}

	staticRoot, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
	http.Handle("/", http.FileServer(http.FS(staticRoot)))
	
	http.HandleFunc("/interactions", handleInteractions(db))
	http.HandleFunc("/subscribe", handleSubscribe(db))
	http.HandleFunc("/vapid-public-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"publicKey": vapidPublicKey})
	})

	addr := fmt.Sprintf("%s:%d", *address, *port)
	log.Printf("Server listening on %s", addr)
	log.Printf("Server hostname: %s", serverHostname)
	log.Fatal(http.ListenAndServe(addr, nil))
}

func initDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS interactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		message TEXT NOT NULL,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS subscriptions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		endpoint TEXT NOT NULL UNIQUE,
		p256dh TEXT NOT NULL,
		auth TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS config (
		key TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	`
	_, err := db.Exec(query)
	return err
}

func initVAPID(db *sql.DB) error {
	// Try to load keys
	row := db.QueryRow("SELECT value FROM config WHERE key = 'vapid_private_key'")
	err := row.Scan(&vapidPrivateKey)
	if err == sql.ErrNoRows {
		// Generate new keys
		privateKey, publicKey, err := webpush.GenerateVAPIDKeys()
		if err != nil {
			return err
		}
		vapidPrivateKey = privateKey
		vapidPublicKey = publicKey

		_, err = db.Exec("INSERT INTO config (key, value) VALUES ('vapid_private_key', ?), ('vapid_public_key', ?)", vapidPrivateKey, vapidPublicKey)
		if err != nil {
			return err
		}
	} else if err != nil {
		return err
	} else {
		// Loaded private key, get public key
		row = db.QueryRow("SELECT value FROM config WHERE key = 'vapid_public_key'")
		if err := row.Scan(&vapidPublicKey); err != nil {
			return err
		}
	}
	log.Printf("VAPID Public Key: %s", vapidPublicKey)
	return nil
}

func handleInteractions(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			rows, err := db.Query("SELECT id, message, timestamp FROM interactions ORDER BY timestamp ASC")
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var interactions []Interaction
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Message, &i.Timestamp); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				interactions = append(interactions, i)
			}
			if interactions == nil {
				interactions = []Interaction{}
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(interactions)

		} else if r.Method == http.MethodPost {
			var i Interaction
			if err := json.NewDecoder(r.Body).Decode(&i); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			res, err := db.Exec("INSERT INTO interactions (message) VALUES (?)", i.Message)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			id, _ := res.LastInsertId()
			i.ID = id
			
			// Trigger Push
			go sendPushNotifications(db, i.Message)

			w.WriteHeader(http.StatusCreated)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func handleSubscribe(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		
		var sub webpush.Subscription
		if err := json.NewDecoder(r.Body).Decode(&sub); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		_, err := db.Exec("INSERT OR IGNORE INTO subscriptions (endpoint, p256dh, auth) VALUES (?, ?, ?)", sub.Endpoint, sub.Keys.P256dh, sub.Keys.Auth)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusCreated)
	}
}

func sendPushNotifications(db *sql.DB, message string) {
	log.Printf("Sending push notifications for message: %s", message)
	rows, err := db.Query("SELECT endpoint, p256dh, auth FROM subscriptions")
	if err != nil {
		log.Printf("Error querying subscriptions: %v", err)
		return
	}
	defer rows.Close()

	count := 0
	for rows.Next() {
		count++
		var sub webpush.Subscription
		if err := rows.Scan(&sub.Endpoint, &sub.Keys.P256dh, &sub.Keys.Auth); err != nil {
			log.Printf("Error scanning subscription: %v", err)
			continue
		}

		// Send Notification
		resp, err := webpush.SendNotification([]byte(message), &sub, &webpush.Options{
			Subscriber:      fmt.Sprintf("mailto:push@%s", serverHostname),
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
			TTL:             30,
		})
		if err != nil {
			log.Printf("Failed to send push to %s: %v", sub.Endpoint, err)
			// Optional: Remove invalid subscriptions (404/410)
		} else {
			defer resp.Body.Close()
			log.Printf("Sent push to %s (Status: %s)", sub.Endpoint, resp.Status)
		}
	}
	log.Printf("Processed %d subscriptions", count)
}
