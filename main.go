package main

import (
	"bufio"
	"bytes"
	"database/sql"
	"embed"
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/SherClockHolmes/webpush-go"
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
			// Buffer full, skip this subscriber
		}
	}
}

var broadcaster = &Broadcaster{
	subscribers: make(map[chan Interaction]bool),
}

func main() {
	defaultHostname, _ := os.Hostname()
	address := flag.String("address", "127.0.0.1:8089", "Address and port to listen on (e.g., 127.0.0.1:8089)")
	dbPath := flag.String("database", "./push.sqlite", "DATABASE")
	hostname := flag.String("hostname", defaultHostname, "HOSTNAME for push notifications")
	resetVapid := flag.Bool("reset-vapid", false, "Reset VAPID keys")
	message := flag.String("m", "", "Message to send (client mode)")
	title := flag.String("t", "", "Title of the message (client mode)")
	appTitle := flag.String("application-title", "", "Custom title for the web application")
	iconPath := flag.String("icon", "", "Path to a PNG file for custom application icons")
	staticOutput := flag.String("static-output", "", "Output directory for the static web app content")
	interactive := flag.Bool("interactive", false, "Enable interactive mode (allow sending messages from the web app)")
	cliService := flag.String("cli-service", "", "Enable interactive CLI mode (modes: text, json, jsonr, tmux)")
	tmuxTarget := flag.String("tmux-target", "", "Target tmux pane for 'tmux' mode (e.g., session:window.pane)")
	flag.Parse()

	if *cliService != "" {
		runCliClient(*address, *cliService, *tmuxTarget)
		return
	}

	if *message != "" {
		url := fmt.Sprintf("http://%s/interactions", *address)
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

	log.Printf("Server listening on %s", *address)
	log.Printf("Server hostname: %s", serverHostname)
	log.Fatal(http.ListenAndServe(*address, nil))
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
		log.Printf("initVAPID: Generated new VAPID keys")
	} else if err != nil {
		return err
	} else {
		// Loaded private key, get public key
		row = db.QueryRow("SELECT value FROM config WHERE key = 'vapid_public_key'")
		if err := row.Scan(&vapidPublicKey); err != nil {
			return err
		}
		log.Printf("initVAPID: Loaded existing VAPID keys")
	}
	log.Printf("initVAPID: Public Key: %s", vapidPublicKey)
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
	i.Timestamp = time.Now().UTC()

	// Trigger Push for non-user messages only
	if !i.IsUser {
		go sendPushNotifications(db, i.Title, i.Message, i.Link)
	}
	// Broadcast for streaming to all clients
	broadcaster.Broadcast(*i)
	return nil
}

func runCliClient(address string, mode string, tmuxTarget string) {
	var clientID string
	if strings.HasPrefix(mode, "tmux:") {
		clientID = strings.TrimPrefix(mode, "tmux:")
		mode = "tmux"
	}

	if mode == "tmux" && tmuxTarget == "" {
		log.Fatal("tmux mode requires --tmux-target")
	}

	needsPrompt := mode == "text" || mode == "" || mode == "jsonr"

	sendMsg := func(text string, title string) {
		i := Interaction{Message: text, Title: title}
		data, _ := json.Marshal(i)
		resp, err := http.Post(fmt.Sprintf("http://%s/service?stream=false", address), "application/x-ndjson", bytes.NewReader(append(data, '\n')))
		if err == nil {
			resp.Body.Close()
		}
	}

	if mode == "tmux" {
		msg := fmt.Sprintf("Now forwarding responses to %s", tmuxTarget)
		if clientID != "" {
			msg += fmt.Sprintf(" (Client ID: %s)", clientID)
		}
		sendMsg(msg, "tmux-service")
		defer sendMsg("No longer forwarding responses", "tmux-service")
	}

	// Receiver: Stream from GET /service
	go func() {
		backoff := 1 * time.Second
		var lastTimestamp time.Time
		for {
			url := fmt.Sprintf("http://%s/service", address)
			if !lastTimestamp.IsZero() {
				url += "?timestamp=" + lastTimestamp.Format(time.RFC3339)
			}
			resp, err := http.Get(url)
			if err != nil {
				fmt.Fprintf(os.Stderr, "\rConnection failed: %v. Retrying in %v...\n", err, backoff)
				if needsPrompt {
					fmt.Print("> ")
				}
				time.Sleep(backoff)
				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}
			backoff = 1 * time.Second // Reset backoff on success

			dec := json.NewDecoder(resp.Body)
			for {
				var i Interaction
				if err := dec.Decode(&i); err != nil {
					resp.Body.Close()
					if err == io.EOF {
						fmt.Fprintf(os.Stderr, "\rConnection closed by server. Reconnecting...\n")
					} else {
						fmt.Fprintf(os.Stderr, "\rStream error: %v. Reconnecting...\n", err)
					}
					if needsPrompt {
						fmt.Print("> ")
					}
					break
				}
				if i.ID == 0 {
					continue // Heartbeat
				}
				if i.Timestamp.After(lastTimestamp) {
					lastTimestamp = i.Timestamp
				}

				if mode == "json" || mode == "jsonr" {
					data, _ := json.Marshal(i)
					fmt.Printf("%s\n", string(data))
				} else if mode == "tmux" {
					if i.IsUser {
						msg := i.Message
						if clientID != "" {
							if strings.HasPrefix(msg, clientID+": ") {
								msg = strings.TrimPrefix(msg, clientID+": ")
							} else if strings.HasPrefix(msg, clientID+" ") {
								msg = strings.TrimPrefix(msg, clientID+" ")
							} else {
								continue // Ignore messages not matching clientID
							}
						}

						// Forward messages from the web app (user) to tmux
						// Send the message
						cmd := exec.Command("tmux", "send-keys", "-t", tmuxTarget, msg)
						if err := cmd.Run(); err != nil {
							fmt.Fprintf(os.Stderr, "\rFailed to send keys to tmux: %v\n", err)
						}
						// Small sleep before Enter
						time.Sleep(100 * time.Millisecond)
						// Send Enter
						cmd = exec.Command("tmux", "send-keys", "-t", tmuxTarget, "Enter")
						if err := cmd.Run(); err != nil {
							fmt.Fprintf(os.Stderr, "\rFailed to send Enter to tmux: %v\n", err)
						}
						if needsPrompt {
							fmt.Print("> ")
						}
					}
				} else {
					author := i.Title
					if author == "" {
						author = "User"
					}
					fmt.Printf("\r[%s] %s: %s\n> ", i.Timestamp.Local().Format("15:04"), author, i.Message)
				}
			}
			time.Sleep(1 * time.Second) // Small delay before reconnecting
		}
	}()

	// Sender: POST to /service?stream=false
	scanner := bufio.NewScanner(os.Stdin)
	if needsPrompt {
		fmt.Print("> ")
	}
	for scanner.Scan() {
		text := scanner.Text()
		if text == "" {
			if needsPrompt {
				fmt.Print("> ")
			}
			continue
		}

		var i Interaction
		if mode == "json" {
			if err := json.Unmarshal([]byte(text), &i); err != nil {
				fmt.Fprintf(os.Stderr, "Invalid JSON input: %v\n", err)
				if needsPrompt {
					fmt.Print("> ")
				}
				continue
			}
		} else {
			i = Interaction{Message: text}
		}

		data, _ := json.Marshal(i)
		resp, err := http.Post(fmt.Sprintf("http://%s/service?stream=false", address), "application/x-ndjson", bytes.NewReader(append(data, '\n')))
		if err == nil {
			resp.Body.Close()
		} else {
			fmt.Fprintf(os.Stderr, "\rSend error: %v\n", err)
		}

		if needsPrompt {
			fmt.Print("> ")
		}
	}
}

func handleService(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "Streaming not supported", http.StatusInternalServerError)
			return
		}

		if r.Method == http.MethodPost && r.URL.Query().Get("stream") == "false" {
			dec := json.NewDecoder(r.Body)
			var i Interaction
			if err := dec.Decode(&i); err == nil {
				i.IsUser = false
				saveInteraction(db, &i)
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.Header().Set("Content-Type", "application/x-ndjson")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.WriteHeader(http.StatusOK)
		flusher.Flush()

		incoming := make(chan Interaction)
		go func() {
			defer close(incoming)
			dec := json.NewDecoder(r.Body)
			for {
				var i Interaction
				if err := dec.Decode(&i); err != nil {
					return
				}
				incoming <- i
			}
		}()

		startTime := time.Now().UTC()
		if tsStr := r.URL.Query().Get("timestamp"); tsStr != "" {
			if ts, err := time.Parse(time.RFC3339, tsStr); err == nil {
				startTime = ts.UTC()
			} else if ts, err := time.Parse("2006-01-02 15:04:05", tsStr); err == nil {
				startTime = ts.UTC()
			}
		}

		ch := broadcaster.Subscribe()
		defer broadcaster.Unsubscribe(ch)

		sentIds := make(map[int64]bool)
		rows, err := db.Query("SELECT id, title, message, detailed_message, link, is_user, timestamp FROM interactions WHERE timestamp > ? ORDER BY id ASC", startTime)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Timestamp); err == nil {
					sentIds[i.ID] = true
					json.NewEncoder(w).Encode(i)
					flusher.Flush()
				}
			}
		}

		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case i, ok := <-ch:
				if !ok {
					return
				}
				if !sentIds[i.ID] {
					if err := json.NewEncoder(w).Encode(i); err != nil {
						return
					}
					flusher.Flush()
				}
			case i, ok := <-incoming:
				if !ok {
					incoming = nil
					continue
				}
				i.IsUser = false
				saveInteraction(db, &i)
			case <-ticker.C:
				w.Write([]byte("{}\n"))
				flusher.Flush()
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
			Subscriber:      "admin@example.com",
			VAPIDPublicKey:  vapidPublicKey,
			VAPIDPrivateKey: vapidPrivateKey,
			VapidExpiration: time.Now().Add(45 * time.Minute),
			TTL:             30,
			HTTPClient: &http.Client{
				Timeout: 30 * time.Second,
			},
		})
		if err != nil {
			log.Printf("Failed to send push to %s: %v", sub.Endpoint, err)
		} else {
			defer resp.Body.Close()
			if resp.StatusCode >= 400 {
				body, _ := io.ReadAll(resp.Body)
				log.Printf("Failed to send push to %s (Status: %s): %s", sub.Endpoint, resp.Status, string(body))
			} else {
				log.Printf("Sent push to %s (Status: %s)", sub.Endpoint, resp.Status)
			}
		}
	}
	log.Printf("Processed %d subscriptions", count)
}
