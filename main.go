package main

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"math/big"
	"net/http"
	"os"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed static/*
var staticFS embed.FS

type Interaction struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Message         string    `json:"message"`
	DetailedMessage string    `json:"detailed_message"`
	Link            string    `json:"link"`
	Timestamp       time.Time `json:"timestamp"`
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
	resetVapid := flag.Bool("reset-vapid", false, "Reset VAPID keys")
	flag.Parse()

	serverHostname = *hostname

	db, err := sql.Open("sqlite3", *dbPath)
	if err != nil {
		log.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	if *resetVapid {
		if _, err := db.Exec("DELETE FROM config WHERE key IN ('vapid_private_key', 'vapid_public_key')"); err != nil {
			log.Fatalf("Failed to reset VAPID keys: %v", err)
		}
		log.Println("VAPID keys reset.")
	}

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
		title TEXT DEFAULT '',
		message TEXT NOT NULL,
		detailed_message TEXT DEFAULT '',
		link TEXT DEFAULT '',
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
	
	// Add columns if they don't exist (migration)
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN title TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN link TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN detailed_message TEXT DEFAULT ''")
	
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
			limit := 100
			if l := r.URL.Query().Get("limit"); l != "" {
				fmt.Sscanf(l, "%d", &limit)
			}

			var rows *sql.Rows
			var err error
			var isHistory bool

			if after := r.URL.Query().Get("after"); after != "" {
				// Polling for new messages
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, timestamp FROM interactions WHERE id > ? ORDER BY id ASC", after)
			} else if before := r.URL.Query().Get("before"); before != "" {
				// Loading history (fetching older messages)
				isHistory = true
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, timestamp FROM interactions WHERE id < ? ORDER BY id DESC LIMIT ?", before, limit)
			} else {
				// Initial load (latest messages)
				isHistory = true
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, timestamp FROM interactions ORDER BY id DESC LIMIT ?", limit)
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var interactions []Interaction
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.Timestamp); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				interactions = append(interactions, i)
			}

			// If we fetched history (DESC), we need to reverse to ASC
			if isHistory {
				for i, j := 0, len(interactions)-1; i < j; i, j = i+1, j-1 {
					interactions[i], interactions[j] = interactions[j], interactions[i]
				}
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

			res, err := db.Exec("INSERT INTO interactions (title, message, detailed_message, link) VALUES (?, ?, ?, ?)", i.Title, i.Message, i.DetailedMessage, i.Link)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			id, _ := res.LastInsertId()
			i.ID = id
			
			// Trigger Push
			go sendPushNotifications(db, i.Title, i.Message, i.Link)

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

type VAPIDTransport struct {
	PrivateKey string
	PublicKey  string
	Subject    string
	Transport  http.RoundTripper
}

func (t *VAPIDTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	origin := fmt.Sprintf("%s://%s", req.URL.Scheme, req.URL.Host)
	header, err := generateVAPIDHeader(t.Subject, origin, t.PrivateKey, t.PublicKey)
	if err != nil {
		return nil, fmt.Errorf("failed to generate VAPID header: %v", err)
	}
	req.Header.Set("Authorization", header)
	if t.Transport == nil {
		return http.DefaultTransport.RoundTrip(req)
	}
	return t.Transport.RoundTrip(req)
}

func generateVAPIDHeader(sub, aud, privateKeyStr, publicKeyStr string) (string, error) {
	// Decode private key
	keyBytes, err := base64.RawURLEncoding.DecodeString(privateKeyStr)
	if err != nil {
		return "", err
	}
	
	priv := new(ecdsa.PrivateKey)
	priv.PublicKey.Curve = elliptic.P256()
	priv.D = new(big.Int).SetBytes(keyBytes)
	priv.PublicKey.X, priv.PublicKey.Y = priv.PublicKey.Curve.ScalarBaseMult(keyBytes)

	// Create JWT
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"aud": aud,
		"exp": time.Now().Add(time.Minute * 20).Unix(),
		"sub": sub,
	})

	tokenString, err := token.SignedString(priv)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("vapid t=%s, k=%s", tokenString, publicKeyStr), nil
}

func sendPushNotifications(db *sql.DB, title, message, link string) {
	log.Printf("Sending push notifications for [%s]: %s (Link: %s)", title, message, link)
	
	payload, _ := json.Marshal(map[string]string{
		"title":   title,
		"message": message,
		"link":    link,
	})

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
		resp, err := webpush.SendNotification(payload, &sub, &webpush.Options{
			Subscriber: "mailto:admin@example.com",
			TTL:        30,
			HTTPClient: &http.Client{
				Transport: &VAPIDTransport{
					PrivateKey: vapidPrivateKey,
					PublicKey:  vapidPublicKey,
					Subject:    "mailto:admin@example.com",
					Transport:  http.DefaultTransport,
				},
				Timeout: 30 * time.Second,
			},
		})
		if err != nil {
			log.Printf("Failed to send push to %s: %v", sub.Endpoint, err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusCreated {
				body, _ := io.ReadAll(resp.Body)
				log.Printf("Failed to send push to %s (Status: %s): %s", sub.Endpoint, resp.Status, string(body))
			} else {
				log.Printf("Sent push to %s (Status: %s)", sub.Endpoint, resp.Status)
			}
		}
	}
	log.Printf("Processed %d subscriptions", count)
}
