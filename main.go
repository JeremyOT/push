package main

import (
	"bufio"
	"bytes"
	"context"
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
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"github.com/SherClockHolmes/webpush-go"
	_ "github.com/mattn/go-sqlite3"
	"golang.org/x/image/draw"
)

//go:embed static/*
var staticFS embed.FS

type Interaction struct {
	ID              int64     `json:"id"`
	Identifier      string    `json:"identifier,omitempty"`
	Title           string    `json:"title"`
	Message         string    `json:"message"`
	DetailedMessage string    `json:"detailed_message"`
	Link            string    `json:"link"`
	IsUser          bool      `json:"is_user"`
	Quiet           bool      `json:"quiet"`
	Timestamp       time.Time `json:"timestamp"`
	Update          bool      `json:"update,omitempty"`
	Replace         bool      `json:"replace,omitempty"`
	Status          string    `json:"status,omitempty"`
	Agent           string    `json:"agent,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
}

var vapidPrivateKey string
var vapidPublicKey string
var serverHostname string
var customIcons = make(map[string][]byte)
var activeSessions = make(map[string]int)
var sessionsMu sync.Mutex

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
	sessionID := flag.String("session-id", "", "Session ID for the CLI service")
	sessionName := flag.String("session-name", "", "Session name (display title) for the CLI service")
	modelName := flag.String("model", "", "Model name for the CLI service")
	flag.Parse()

	if *cliService != "" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			cancel()
		}()
		runCliClient(ctx, *address, *cliService, *tmuxTarget, *sessionID, *sessionName, *modelName, os.Stdin, os.Stdout, os.Stderr)
		return
	}

	if *message != "" {
		if err := sendMessage(*address, *message, *title); err != nil {
			log.Fatalf("Failed to send message: %v", err)
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
	queries := []string{
		`CREATE TABLE IF NOT EXISTS interactions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			identifier TEXT DEFAULT '',
			title TEXT DEFAULT '',
			message TEXT NOT NULL,
			detailed_message TEXT DEFAULT '',
			link TEXT DEFAULT '',
			is_user BOOLEAN DEFAULT 0,
			quiet BOOLEAN DEFAULT 0,
			timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
			status TEXT DEFAULT '',
			agent TEXT DEFAULT '',
			session_id TEXT DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS subscriptions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			endpoint TEXT NOT NULL UNIQUE,
			p256dh TEXT NOT NULL,
			auth TEXT NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS config (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL
		)`,
	}

	for _, q := range queries {
		if _, err := db.Exec(q); err != nil {
			return err
		}
	}

	// Add columns if they don't exist (migration)
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN identifier TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN title TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN link TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN detailed_message TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN is_user BOOLEAN DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN quiet BOOLEAN DEFAULT 0")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN status TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN agent TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN session_id TEXT DEFAULT ''")

	return nil
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
				rows, err = db.Query("SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, agent, session_id FROM interactions WHERE id > ? ORDER BY id ASC", after)
			} else if before := r.URL.Query().Get("before"); before != "" {
				// Loading history (fetching older messages)
				isHistory = true
				rows, err = db.Query("SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, agent, session_id FROM interactions WHERE id < ? ORDER BY id DESC LIMIT ?", before, limit)
			} else {
				// Initial load (latest messages)
				isHistory = true
				rows, err = db.Query("SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, agent, session_id FROM interactions ORDER BY id DESC LIMIT ?", limit)
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var interactions []Interaction
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Identifier, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Quiet, &i.Timestamp, &i.Status, &i.Agent, &i.SessionID); err != nil {
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

			// Special handling for /run command to trigger deploy
			if i.IsUser && strings.HasPrefix(strings.TrimSpace(i.Message), "/run") {
				go func() {
					log.Printf("Executing deploy.sh via /run command")
					cmd := exec.Command("./deploy.sh")
					// Start and forget, it handles its own logging/backgrounding
					_ = cmd.Start()
				}()
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
	if i.Identifier != "" {
		// Check if it already exists
		var id int64
		var timestamp time.Time
		var existingTitle string
		var existingMessage string
		var existingDetailedMessage string
		var existingLink string
		var existingStatus string
		var existingAgent string
		var existingSessionID string
		var existingIsUser bool
		var existingQuiet bool
		err := db.QueryRow("SELECT id, timestamp, title, message, detailed_message, link, status, agent, session_id, is_user, quiet FROM interactions WHERE identifier = ?", i.Identifier).Scan(&id, &timestamp, &existingTitle, &existingMessage, &existingDetailedMessage, &existingLink, &existingStatus, &existingAgent, &existingSessionID, &existingIsUser, &existingQuiet)
		if err == nil {
			// Exists, update it
			if i.Title == "" {
				i.Title = existingTitle
			}
			if i.Link == "" {
				i.Link = existingLink
			}
			if i.Status == "" {
				i.Status = existingStatus
			}
			if i.Agent == "" {
				i.Agent = existingAgent
			}
			if i.SessionID == "" {
				i.SessionID = existingSessionID
			}
			// For boolean fields, we only merge if the new value is false and existing was true?
			// Actually, it's better to just check if they were provided in JSON.
			// But Go unmarshals missing bools as false.
			// Given the current hooks, we can assume if they are true, they should stay true?
			// No, that's not right.
			// Let's assume for now that if we are updating by identifier, we want to keep the existing is_user.
			i.IsUser = existingIsUser
			// For quiet, we might want to update it. aftermodel.py sends it.
			// If it's missing in the update request, we might want to keep it.
			// But how to detect if it's missing? We can't easily with the current struct.
			// Let's just merge it if it was not in the request? 
			// For now, let's just make sure we are not accidentally clearing it if it was true.
			if !i.Quiet && existingQuiet {
				// i.Quiet = existingQuiet // Only if we want to preserve "quiet"
			}
			if !i.Replace {
				i.Message = existingMessage + i.Message
				i.DetailedMessage = existingDetailedMessage + i.DetailedMessage
			}
			_, err = db.Exec("UPDATE interactions SET title = ?, message = ?, detailed_message = ?, link = ?, is_user = ?, quiet = ?, status = ?, agent = ?, session_id = ? WHERE id = ?", i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Agent, i.SessionID, id)
			if err != nil {
				return err
			}
			i.ID = id
			i.Timestamp = timestamp
			i.Update = true
		} else if err == sql.ErrNoRows {
			// Not found, insert new
			res, err := db.Exec("INSERT INTO interactions (identifier, title, message, detailed_message, link, is_user, quiet, status, agent, session_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", i.Identifier, i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Agent, i.SessionID)
			if err != nil {
				return err
			}
			id, _ := res.LastInsertId()
			i.ID = id
			i.Timestamp = time.Now().UTC()
		} else {
			return err
		}
	} else {
		res, err := db.Exec("INSERT INTO interactions (identifier, title, message, detailed_message, link, is_user, quiet, status, agent, session_id) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "", i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Agent, i.SessionID)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		i.ID = id
		i.Timestamp = time.Now().UTC()
	}

	// Trigger Push for non-user messages only, and only if not quiet and not an update
	if !i.IsUser && !i.Quiet && !i.Update {
		go sendPushNotifications(db, i.Title, i.Message, i.Link)
	}
	// Broadcast for streaming to all clients
	broadcaster.Broadcast(*i)
	return nil
}

func sendMessage(address, message, title string) error {
	url := fmt.Sprintf("http://%s/interactions", address)
	payload := map[string]string{
		"message": message,
		"title":   title,
	}
	jsonPayload, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(jsonPayload))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned error: %s - %s", resp.Status, string(body))
	}
	return nil
}

func isTerminal(r io.Reader) bool {
	if f, ok := r.(*os.File); ok {
		var sz struct {
			rows, cols, xpixel, ypixel uint16
		}
		_, _, err := syscall.Syscall(syscall.SYS_IOCTL,
			f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&sz)))
		return err == 0
	}
	return false
}

func runCliClient(ctx context.Context, address string, mode string, tmuxTarget string, sessionID string, sessionName string, model string, stdin io.Reader, stdout io.Writer, stderr io.Writer) {
	var clientID string
	if strings.HasPrefix(mode, "tmux:") {
		clientID = strings.TrimPrefix(mode, "tmux:")
		mode = "tmux"
	}

	if mode == "tmux" {
		if tmuxTarget == "" {
			fmt.Fprintln(stderr, "tmux mode requires --tmux-target")
			return
		}
		if _, err := exec.LookPath("tmux"); err != nil {
			fmt.Fprintf(stderr, "tmux mode requires tmux to be installed and in PATH: %v\n", err)
			return
		}
	}

	needsPrompt := mode == "text" || mode == "" || mode == "jsonr"

	title := sessionName
	if title == "" {
		title = "CLI Agent"
	}
	agent := "remote"
	if model != "" {
		if strings.Contains(strings.ToLower(model), "gemini") {
			agent = "gemini"
		} else if strings.Contains(strings.ToLower(model), "claude") {
			agent = "claude"
		}
	}

	sendMsg := func(text string, title string, agent string, status string) {
		i := Interaction{Message: text, Title: title, SessionID: sessionID, Agent: agent, Status: status}
		data, _ := json.Marshal(i)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(fmt.Sprintf("http://%s/service?stream=false", address), "application/x-ndjson", bytes.NewReader(append(data, '\n')))
		if err == nil {
			resp.Body.Close()
		} else {
			fmt.Fprintf(stderr, "\rFailed to notify service: %v\n", err)
		}
	}

	if mode == "tmux" {
		defer func() {
			exitMsg := "No longer forwarding responses"
			if clientID != "" {
				exitMsg += fmt.Sprintf(" (Client ID: %s)", clientID)
			}
			sendMsg(exitMsg, "tmux-service", "", "")
			time.Sleep(100 * time.Millisecond) // Give the exit message a moment
		}()
	}

	// Receiver: Stream from GET /service
	go func() {
		time.Sleep(100 * time.Millisecond) // Give the sender a head start
		backoff := 1 * time.Second
		var lastTimestamp time.Time
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			url := fmt.Sprintf("http://%s/service", address)
			params := []string{}
			if !lastTimestamp.IsZero() {
				params = append(params, "timestamp="+lastTimestamp.Format(time.RFC3339))
			}
			if sessionID != "" {
				params = append(params, "session_id="+sessionID)
			}
			if len(params) > 0 {
				url += "?" + strings.Join(params, "&")
			}
			
			req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				if ctx.Err() != nil {
					return
				}
				fmt.Fprintf(stderr, "\rConnection failed: %v. Retrying in %v...\n", err, backoff)
				if needsPrompt {
					fmt.Fprint(stdout, "> ")
				}

				select {
				case <-time.After(backoff):
				case <-ctx.Done():
					return
				}

				backoff *= 2
				if backoff > 30*time.Second {
					backoff = 30 * time.Second
				}
				continue
			}
			backoff = 1 * time.Second // Reset backoff on success

			// Re-register session and active services on successful connection
			if sessionID != "" {
				sendMsg(fmt.Sprintf("Registered session: %s", title), "session-register", agent, "d")
			}
			if mode == "tmux" {
				msg := fmt.Sprintf("Now forwarding responses to %s", tmuxTarget)
				if clientID != "" {
					msg += fmt.Sprintf(" (Client ID: %s)", clientID)
				}
				sendMsg(msg, "tmux-service", "", "")
			}

			dec := json.NewDecoder(resp.Body)
			for {
				var i Interaction
				if err := dec.Decode(&i); err != nil {
					resp.Body.Close()
					if ctx.Err() != nil {
						return
					}
					if err == io.EOF {
						fmt.Fprintf(stderr, "\rConnection closed by server. Reconnecting...\n")
					} else {
						fmt.Fprintf(stderr, "\rStream error: %v. Reconnecting...\n", err)
					}
					if needsPrompt {
						fmt.Fprint(stdout, "> ")
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
					fmt.Fprintf(stdout, "%s\n", string(data))
				} else if mode == "tmux" {
					if i.IsUser {
						msg := i.Message
						if clientID != "" {
							prefix1 := clientID + ": "
							prefix2 := clientID + " "
							if strings.HasPrefix(strings.ToLower(msg), strings.ToLower(prefix1)) {
								msg = msg[len(prefix1):]
							} else if strings.HasPrefix(strings.ToLower(msg), strings.ToLower(prefix2)) {
								msg = msg[len(prefix2):]
							} else {
								continue // Ignore messages not matching clientID
							}
						}

						// Forward messages from the web app (user) to tmux
						// Send the message
						cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", tmuxTarget, msg)
						if err := cmd.Run(); err != nil {
							fmt.Fprintf(stderr, "\rFailed to send keys to tmux: %v (Target: %s)\n", err, tmuxTarget)
						}
						// Small sleep before Enter
						time.Sleep(100 * time.Millisecond)
						// Send Enter
						cmd = exec.CommandContext(ctx, "tmux", "send-keys", "-t", tmuxTarget, "Enter")
						if err := cmd.Run(); err != nil {
							fmt.Fprintf(stderr, "\rFailed to send Enter to tmux: %v (Target: %s)\n", err, tmuxTarget)
						}
						if needsPrompt {
							fmt.Fprint(stdout, "> ")
						}
					}
				} else {
					author := i.Agent
					if author == "" {
						author = i.Title
					}
					if author == "" {
						author = "User"
					}
					status := ""
					if i.Status == "w" {
						status = " (Working)"
					} else if i.Status == "d" {
						status = " (Done)"
					} else if i.Status == "r" {
						status = " (Ready)"
					}
					fmt.Fprintf(stdout, "\r[%s] %s%s: %s\n> ", i.Timestamp.Local().Format("15:04"), author, status, i.Message)
				}
			}

			select {
			case <-time.After(1 * time.Second):
			case <-ctx.Done():
				return
			}
		}
	}()

	// Sender: POST to /service?stream=false
	inputChan := make(chan string)
	go func() {
		scanner := bufio.NewScanner(stdin)
		for scanner.Scan() {
			inputChan <- scanner.Text()
		}
		if err := scanner.Err(); err != nil {
			fmt.Fprintf(stderr, "Stdin error: %v\n", err)
		}
		close(inputChan)
	}()

	if needsPrompt {
		fmt.Fprint(stdout, "> ")
	}

loop:
	for {
		select {
		case <-ctx.Done():
			break loop
		case text, ok := <-inputChan:
			if !ok {
				// Stdin closed (Ctrl-D)
				if mode == "tmux" {
					if !isTerminal(stdin) {
						// Input is redirected or backgrounded, not a terminal.
						// Block indefinitely to keep receiving and forwarding.
						<-ctx.Done()
						break loop
					}
				}
				break loop
			}

			if text == "" {
				if needsPrompt {
					fmt.Fprint(stdout, "> ")
				}
				continue
			}

			var i Interaction
			if mode == "json" {
				if err := json.Unmarshal([]byte(text), &i); err != nil {
					fmt.Fprintf(stderr, "Invalid JSON input: %v\n", err)
					if needsPrompt {
						fmt.Fprint(stdout, "> ")
					}
					continue
				}
			} else {
				i = Interaction{Message: text, Agent: agent, Title: title}
			}

			if i.SessionID == "" {
				i.SessionID = sessionID
			}

			data, _ := json.Marshal(i)
			req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/service?stream=false", address), bytes.NewReader(append(data, '\n')))
			req.Header.Set("Content-Type", "application/x-ndjson")
			resp, err := http.DefaultClient.Do(req)
			if err == nil {
				resp.Body.Close()
			} else {
				fmt.Fprintf(stderr, "\rSend error: %v\n", err)
			}

			if needsPrompt {
				fmt.Fprint(stdout, "> ")
			}
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

		sessionID := r.URL.Query().Get("session_id")
		if sessionID != "" {
			sessionsMu.Lock()
			activeSessions[sessionID]++
			if activeSessions[sessionID] == 1 {
				go broadcaster.Broadcast(Interaction{
					Title:     "session-active",
					SessionID: sessionID,
				})
			}
			sessionsMu.Unlock()

			defer func() {
				sessionsMu.Lock()
				activeSessions[sessionID]--
				if activeSessions[sessionID] <= 0 {
					delete(activeSessions, sessionID)
					go broadcaster.Broadcast(Interaction{
						Title:     "session-inactive",
						SessionID: sessionID,
					})
				}
				sessionsMu.Unlock()
			}()
		}

		if r.Method == http.MethodPost && r.URL.Query().Get("stream") == "false" {
			dec := json.NewDecoder(r.Body)
			var i Interaction
			if err := dec.Decode(&i); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			i.IsUser = false
			if i.SessionID == "" {
				i.SessionID = sessionID
			}
			saveInteraction(db, &i)
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
				i.IsUser = false
				if i.SessionID == "" {
					i.SessionID = sessionID
				}
				saveInteraction(db, &i)
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

		// Send initial heartbeat with active sessions
		sessionsMu.Lock()
		var initialActive []string
		for sid := range activeSessions {
			initialActive = append(initialActive, sid)
		}
		sessionsMu.Unlock()
		json.NewEncoder(w).Encode(Interaction{
			Title:   "heartbeat",
			Message: strings.Join(initialActive, ","),
		})
		flusher.Flush()

		sentIds := make(map[int64]bool)
		query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, agent, session_id FROM interactions WHERE timestamp > ?"
		args := []interface{}{startTime}
		if sessionID != "" {
			query += " AND (session_id = ? OR session_id = '')"
			args = append(args, sessionID)
		}
		query += " ORDER BY id ASC"

		rows, err := db.Query(query, args...)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Identifier, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Quiet, &i.Timestamp, &i.Status, &i.Agent, &i.SessionID); err == nil {
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
				// Filter: if client has sessionID, only send matches or global messages.
				// If no sessionID, send everything (main feed).
				if sessionID != "" && i.SessionID != "" && i.SessionID != sessionID {
					continue
				}

				if !sentIds[i.ID] {
					if err := json.NewEncoder(w).Encode(i); err != nil {
						return
					}
					flusher.Flush()
				}
			case _, ok := <-incoming:
				if !ok {
					incoming = nil
					continue
				}
				// Handled by the goroutine above
			case <-ticker.C:
				sessionsMu.Lock()
				var active []string
				for sid := range activeSessions {
					active = append(active, sid)
				}
				sessionsMu.Unlock()
				msg := Interaction{
					Title:   "heartbeat",
					Message: strings.Join(active, ","),
				}
				json.NewEncoder(w).Encode(msg)
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
