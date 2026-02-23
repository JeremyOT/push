package main

import (
	"bufio"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"database/sql"
	"embed"
	"encoding/base64"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log"
	"math/big"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
	"github.com/golang-jwt/jwt/v5"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/image/draw"
)

//go:embed static/*
var staticFS embed.FS

type Interaction struct {
	ID              int64     `json:"id"`
	Title           string    `json:"title"`
	Message         string    `json:"message"`
	DetailedMessage string    `json:"detailed_message"`
	Link            string    `json:"link"`
	IsUser          bool      `json:"is_user"`
	Timestamp       time.Time `json:"timestamp"`
}

var vapidPrivateKey string
var vapidPublicKey string
var serverHostname string
var customIcons = make(map[string][]byte)

type Broadcaster struct {
	subscribers map[chan Interaction]bool
	mu          sync.Mutex
}

func (b *Broadcaster) Subscribe() chan Interaction {
	b.mu.Lock()
	defer b.mu.Unlock()
	ch := make(chan Interaction, 10)
	b.subscribers[ch] = true
	return ch
}

func (b *Broadcaster) Unsubscribe(ch chan Interaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.subscribers, ch)
	close(ch)
}

func (b *Broadcaster) Broadcast(i Interaction) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subscribers {
		select {
		case ch <- i:
		default:
			// Buffer full, skip this subscriber or drop connection?
			// For now, just skip to avoid blocking.
		}
	}
}

var broadcaster = &Broadcaster{
	subscribers: make(map[chan Interaction]bool),
}

func main() {
	defaultHostname, _ := os.Hostname()
	listenAddr := flag.String("listen", "127.0.0.1:8089", "Address and port to listen on (e.g., 127.0.0.1:8089)")
	dbPath := flag.String("database", "./push.sqlite", "DATABASE")
	hostname := flag.String("hostname", defaultHostname, "HOSTNAME for push notifications")
	resetVapid := flag.Bool("reset-vapid", false, "Reset VAPID keys")
	message := flag.String("m", "", "Message to send (client mode)")
	title := flag.String("t", "", "Title of the message (client mode)")
	appTitle := flag.String("application-title", "", "Custom title for the web application")
	iconPath := flag.String("icon", "", "Path to a PNG file for custom application icons")
	staticOutput := flag.String("static-output", "", "Output directory for the static web app content")
	interactive := flag.Bool("interactive", false, "Enable interactive mode (allow sending messages from the web app)")
	cliService := flag.Bool("cli-service", false, "Use /service endpoint for interactive CLI chat")
	flag.Parse()

	if *cliService {
		url := fmt.Sprintf("http://%s/service", *listenAddr)
		pr, pw := io.Pipe()

		go func() {
			defer pw.Close()
			scanner := bufio.NewScanner(os.Stdin)
			for scanner.Scan() {
				text := scanner.Text()
				if text == "" {
					continue
				}
				i := Interaction{Message: text}
				if err := json.NewEncoder(pw).Encode(i); err != nil {
					log.Printf("Failed to encode message: %v", err)
					return
				}
			}
		}()

		resp, err := http.Post(url, "application/x-ndjson", pr)
		if err != nil {
			log.Fatalf("Failed to connect to service: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Fatalf("Server error: %s - %s", resp.Status, string(body))
		}

		dec := json.NewDecoder(resp.Body)
		fmt.Print("> ")
		for {
			var i Interaction
			if err := dec.Decode(&i); err != nil {
				if err == io.EOF {
					break
				}
				log.Fatalf("Failed to decode response: %v", err)
			}
			author := i.Title
			if author == "" {
				author = "User"
			}
			fmt.Printf("\r[%s] %s: %s\n> ", i.Timestamp.Format("15:04"), author, i.Message)
		}
		return
	}

	if *message != "" {
		url := fmt.Sprintf("http://%s/interactions", *listenAddr)
		payload := map[string]string{
			"message": *message,
			"title":   *title,
		}
		jsonPayload, err := json.Marshal(payload)
		if err != nil {
			log.Fatalf("Failed to marshal payload: %v", err)
		}
		resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
		if err != nil {
			log.Fatalf("Failed to send request: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			log.Fatalf("Server returned error: %s - %s", resp.Status, string(body))
		}
		fmt.Println("Message sent successfully.")
		return
	}

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

	if *iconPath != "" {
		if err := loadCustomIcons(*iconPath); err != nil {
			log.Fatalf("Failed to load custom icons: %v", err)
		}
	}

	if *staticOutput != "" {
		if err := exportStatic(staticRoot, *staticOutput, *appTitle, *iconPath != "", *interactive); err != nil {
			log.Fatalf("Failed to export static content: %v", err)
		}
		log.Printf("Static content exported to %s", *staticOutput)
		return
	}

	http.HandleFunc("/", handleStatic(staticRoot, *appTitle, *iconPath != "", *interactive))
	http.HandleFunc("/interactions", handleInteractions(db))
	http.HandleFunc("/service", handleService(db))
	http.HandleFunc("/subscribe", handleSubscribe(db))
	http.HandleFunc("/vapid-public-key", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"publicKey": vapidPublicKey})
	})

	log.Printf("Server listening on %s", *listenAddr)
	log.Printf("Server hostname: %s", serverHostname)
	log.Fatal(http.ListenAndServe(*listenAddr, nil))
}

func getStaticContent(staticRoot fs.FS, path string, appTitle string, hasCustomIcon bool, interactive bool) ([]byte, string, time.Time, error) {
	path = strings.TrimPrefix(path, "/")
	if path == "" {
		path = "index.html"
	}

	if data, ok := customIcons[path]; ok {
		return data, "image/png", time.Now(), nil
	}

	if hasCustomIcon && path == "icon.svg" {
		if data, ok := customIcons["icon.png"]; ok {
			return data, "image/png", time.Now(), nil
		}
	}

	f, err := staticRoot.Open(path)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	defer f.Close()

	stat, err := f.Stat()
	if err != nil {
		return nil, "", time.Time{}, err
	}

	if (path == "index.html" || path == "manifest.json" || path == "sw.js") && (appTitle != "" || hasCustomIcon || interactive) {
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, "", time.Time{}, err
		}
		content := string(data)
		if path == "index.html" {
			if appTitle != "" {
				content = strings.ReplaceAll(content, "<title>Push</title>", "<title>"+appTitle+"</title>")
				content = strings.ReplaceAll(content, "<h1>Push</h1>", "<h1>"+appTitle+"</h1>")
			}
			if hasCustomIcon {
				content = strings.ReplaceAll(content, "icon.svg", "icon.png")
				content = strings.ReplaceAll(content, "type=\"image/svg+xml\"", "type=\"image/png\"")
			}
			if interactive {
				content = strings.ReplaceAll(content, `{"interactive": false}`, `{"interactive": true}`)
			}
		} else if path == "manifest.json" {
			if appTitle != "" {
				content = strings.ReplaceAll(content, `"name": "Push"`, `"name": "`+appTitle+`"`)
				content = strings.ReplaceAll(content, `"short_name": "Push"`, `"short_name": "`+appTitle+`"`)
			}
			if hasCustomIcon {
				content = strings.ReplaceAll(content, "/icon.svg", "/icon.png")
				content = strings.ReplaceAll(content, "image/svg+xml", "image/png")
			}
		} else if path == "sw.js" {
			if appTitle != "" {
				content = strings.ReplaceAll(content, "let title = 'Push';", "let title = '"+appTitle+"';")
				content = strings.ReplaceAll(content, "title = data.title || 'Push';", "title = data.title || '"+appTitle+"';")
			}
		}
		return []byte(content), mime.TypeByExtension(filepath.Ext(path)), stat.ModTime(), nil
	}

	data, err := io.ReadAll(f)
	if err != nil {
		return nil, "", time.Time{}, err
	}
	return data, mime.TypeByExtension(filepath.Ext(path)), stat.ModTime(), nil
}

func exportStatic(staticRoot fs.FS, outputDir string, appTitle string, hasCustomIcon bool, interactive bool) error {
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return err
	}

	return fs.WalkDir(staticRoot, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return os.MkdirAll(filepath.Join(outputDir, path), 0755)
		}

		data, _, _, err := getStaticContent(staticRoot, path, appTitle, hasCustomIcon, interactive)
		if err != nil {
			return err
		}

		return os.WriteFile(filepath.Join(outputDir, path), data, 0644)
	})
}

func handleStatic(staticRoot fs.FS, appTitle string, hasCustomIcon bool, interactive bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, contentType, modTime, err := getStaticContent(staticRoot, r.URL.Path, appTitle, hasCustomIcon, interactive)
		if err != nil {
			if os.IsNotExist(err) {
				http.NotFound(w, r)
			} else {
				http.Error(w, err.Error(), http.StatusInternalServerError)
			}
			return
		}

		w.Header().Set("Content-Type", contentType)
		http.ServeContent(w, r, r.URL.Path, modTime, bytes.NewReader(data))
	}
}

func loadCustomIcons(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	img, err := png.Decode(f)
	if err != nil {
		return fmt.Errorf("failed to decode PNG: %v", err)
	}

	sizes := map[string]int{
		"icon-128.png":          128,
		"icon-192.png":          192,
		"icon.png":              512,
		"apple-touch-icon.png":  180,
	}

	for name, size := range sizes {
		data, err := resizeImage(img, size)
		if err != nil {
			return fmt.Errorf("failed to resize %s: %v", name, err)
		}
		customIcons[name] = data
	}

	return nil
}

func resizeImage(src image.Image, size int) ([]byte, error) {
	dst := image.NewRGBA(image.Rect(0, 0, size, size))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	var buf bytes.Buffer
	if err := png.Encode(&buf, dst); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func initDB(db *sql.DB) error {
	query := `
	CREATE TABLE IF NOT EXISTS interactions (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		title TEXT DEFAULT '',
		message TEXT NOT NULL,
		detailed_message TEXT DEFAULT '',
		link TEXT DEFAULT '',
		is_user BOOLEAN DEFAULT 0,
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
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN is_user BOOLEAN DEFAULT 0")

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
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, is_user, timestamp FROM interactions WHERE id > ? ORDER BY id ASC", after)
			} else if before := r.URL.Query().Get("before"); before != "" {
				// Loading history (fetching older messages)
				isHistory = true
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, is_user, timestamp FROM interactions WHERE id < ? ORDER BY id DESC LIMIT ?", before, limit)
			} else {
				// Initial load (latest messages)
				isHistory = true
				rows, err = db.Query("SELECT id, title, message, detailed_message, link, is_user, timestamp FROM interactions ORDER BY id DESC LIMIT ?", limit)
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var interactions []Interaction
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Timestamp); err != nil {
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

			if err := saveInteraction(db, &i); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(i)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

func saveInteraction(db *sql.DB, i *Interaction) error {
	res, err := db.Exec("INSERT INTO interactions (title, message, detailed_message, link, is_user) VALUES (?, ?, ?, ?, ?)", i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	i.ID = id
	// Get actual timestamp from DB or just use current
	i.Timestamp = time.Now()

	// Trigger Push
	go sendPushNotifications(db, i.Title, i.Message, i.Link)
	// Broadcast for streaming
	broadcaster.Broadcast(*i)
	return nil
}

func handleService(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		// Handle incoming messages if any
		if r.Body != nil {
			go func() {
				dec := json.NewDecoder(r.Body)
				for {
					var i Interaction
					if err := dec.Decode(&i); err != nil {
						if err != io.EOF {
							log.Printf("Error decoding service message: %v", err)
						}
						return
					}
					if err := saveInteraction(db, &i); err != nil {
						log.Printf("Error saving service interaction: %v", err)
					}
				}
			}()
		}

		timestamp := time.Now()
		if tsStr := r.URL.Query().Get("timestamp"); tsStr != "" {
			if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
				timestamp = ts
			} else if ts, err := time.Parse("2006-01-02 15:04:05", tsStr); err == nil {
				timestamp = ts
			}
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")

		// Send missed messages since timestamp
		rows, err := db.Query("SELECT id, title, message, detailed_message, link, is_user, timestamp FROM interactions WHERE is_user = 1 AND timestamp > ? ORDER BY id ASC", timestamp)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Timestamp); err == nil {
					json.NewEncoder(w).Encode(i)
					flusher.Flush()
				}
			}
		}

		ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(ch)

		for {
			select {
			case i, ok := <-ch:
				if !ok {
					return
				}
				if i.IsUser {
					if err := json.NewEncoder(w).Encode(i); err != nil {
						return
					}
					flusher.Flush()
				}
			case <-r.Context().Done():
				return
			}
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
