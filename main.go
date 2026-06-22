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

//go:embed gemini-agent
var geminiAgentScript string

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
	Kind            string    `json:"kind,omitempty"`
	Agent           string    `json:"agent,omitempty"`
	SessionID       string    `json:"session_id,omitempty"`
	SessionPath     string    `json:"session_path,omitempty"`
}

type AgyToolCall struct {
	Name string          `json:"name"`
	Args json.RawMessage `json:"args"`
}

type AgyLogLine struct {
	ID        string `json:"id"`
	StepIndex *int   `json:"step_index"`
	Type      string `json:"type"`
	Source    string `json:"source"`
	Status    string `json:"status"`
	SessionID string `json:"sessionId"`
	Content   string `json:"content"`
	Thoughts  []struct {
		Subject     string `json:"subject"`
		Description string `json:"description"`
	} `json:"thoughts"`
	Thinking  string         `json:"thinking"`
	Tokens    map[string]int `json:"tokens"`
	Set       interface{}    `json:"$set"`
	ToolCalls []AgyToolCall  `json:"tool_calls"`
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
	// Normalize em-dash (—) and en-dash (–) in command line arguments to standard hyphens
	os.Args = normalizeArgs(os.Args)

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
	sessionPath := flag.String("session-path", "", "Working directory path for the CLI service")
	modelName := flag.String("model", "", "Model name for the CLI service")
	geminiAgent := flag.Bool("gemini-agent", false, "Run the embedded agent script with gemini")
	antigravityAgent := flag.Bool("antigravity", false, "Run the embedded agent script with agy")
	resumeAgent := flag.Bool("resume", false, "Resume the last agent session")
	continueAgent := flag.Bool("continue", false, "Resume the last agent session (alias for -resume when used with -antigravity)")
	yoloAgent := flag.Bool("yolo", false, "Enable YOLO mode (pass appropriate flags to the agent, e.g. -y for gemini, --dangerously-skip-permissions for agy)")
	hermesAgent := flag.String("hermes-agent", "", "URL of the Hermes Agent API for SSE proxy")

	// Internal agy scraper flags
	internalAgyScraper := flag.Bool("internal-agy-scraper", false, "Internal use only: run the agy log scraper")
	agyLogDir := flag.String("agy-log-dir", "", "Internal use only: log directory for agy scraper")
	agyLogFile := flag.String("agy-log-file", "", "Internal use only: specific log file for agy scraper")
	agyBackendURL := flag.String("agy-backend-url", "", "Internal use only: backend URL for agy scraper")
	agyFallbackSessionID := flag.String("agy-fallback-session-id", "", "Internal use only: fallback session ID for agy scraper")
	agySessionPath := flag.String("agy-session-path", "", "Internal use only: session path for agy scraper")

	flag.Parse()

	if *internalAgyScraper {
		runAgyScraper(*agyLogDir, *agyLogFile, *agyBackendURL, *agyFallbackSessionID, *agySessionPath, *yoloAgent)
		return
	}

	if *geminiAgent || *antigravityAgent {
		agentArgs := translateAgentArgs(*antigravityAgent, *resumeAgent || *continueAgent, *yoloAgent, flag.Args())
		runGeminiAgent(agentArgs, *address)
		return
	}

	if *cliService != "" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			cancel()
		}()
		runCliClient(ctx, *address, *cliService, *tmuxTarget, *sessionID, *sessionName, *sessionPath, *modelName, *yoloAgent, os.Stdin, os.Stdout, os.Stderr)
		return
	}

	if *hermesAgent != "" {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigChan
			cancel()
		}()
		runHermesAgent(ctx, *hermesAgent, *address, *sessionID, *sessionName)
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
	http.HandleFunc("/rename-session", handleRenameSession(db))
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
		"icon-128.png":         128,
		"icon-192.png":         192,
		"icon.png":             512,
		"apple-touch-icon.png": 180,
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

func mergeStrings(existing, update string) string {
	if update == "" {
		return existing
	}
	if existing == "" {
		return update
	}

	if strings.Contains(existing, update) {
		return existing
	}

	er := []rune(existing)
	ur := []rune(update)

	// Find the maximal overlap between the end of existing and the start of ur.
	// We want to find the largest k such that er[len(er)-k:] == ur[:k].
	maxOverlap := 0
	limit := len(er)
	if len(ur) < limit {
		limit = len(ur)
	}

	for k := 1; k <= limit; k++ {
		match := true
		for i := 0; i < k; i++ {
			if er[len(er)-k+i] != ur[i] {
				match = false
				break
			}
		}
		if match {
			maxOverlap = k
		}
	}

	return existing + string(ur[maxOverlap:])
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
			kind TEXT DEFAULT '',
			agent TEXT DEFAULT '',
			session_id TEXT DEFAULT '',
			session_path TEXT DEFAULT ''
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
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN kind TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN agent TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN session_id TEXT DEFAULT ''")
	_, _ = db.Exec("ALTER TABLE interactions ADD COLUMN session_path TEXT DEFAULT ''")

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

			sessionID := r.URL.Query().Get("session_id")
			latestPerSession := r.URL.Query().Get("latest_per_session") == "true"

			if latestPerSession {
				// Fetch the latest interaction for each session_id
				query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, kind, agent, session_id, session_path FROM interactions WHERE id IN (SELECT MAX(id) FROM interactions GROUP BY session_id) ORDER BY id ASC"
				rows, err = db.Query(query)
			} else if after := r.URL.Query().Get("after"); after != "" {
				// Polling for new messages
				query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, kind, agent, session_id, session_path FROM interactions WHERE id > ?"
				args := []interface{}{after}
				if sessionID != "" {
					query += " AND session_id = ?"
					args = append(args, sessionID)
				}
				query += " ORDER BY id ASC"
				rows, err = db.Query(query, args...)
			} else if before := r.URL.Query().Get("before"); before != "" {
				// Loading history (fetching older messages)
				isHistory = true
				query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, kind, agent, session_id, session_path FROM interactions WHERE id < ?"
				args := []interface{}{before}
				if sessionID != "" {
					query += " AND session_id = ?"
					args = append(args, sessionID)
				}
				query += " ORDER BY id DESC LIMIT ?"
				args = append(args, limit)
				rows, err = db.Query(query, args...)
			} else {
				// Initial load (latest messages)
				isHistory = true
				query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, kind, agent, session_id, session_path FROM interactions"
				var args []interface{}
				if sessionID != "" {
					query += " WHERE session_id = ?"
					args = append(args, sessionID)
				}
				query += " ORDER BY id DESC LIMIT ?"
				args = append(args, limit)
				rows, err = db.Query(query, args...)
			}

			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			defer rows.Close()

			var interactions []Interaction
			for rows.Next() {
				var i Interaction
				if err := rows.Scan(&i.ID, &i.Identifier, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Quiet, &i.Timestamp, &i.Status, &i.Kind, &i.Agent, &i.SessionID, &i.SessionPath); err != nil {
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
	var existingQuiet bool
	if i.SessionID != "" && (i.Agent == "" || i.SessionPath == "" || i.Title == "" || i.Title == "Gemini" || i.Title == "Antigravity" || i.Title == "Remote" || i.Title == "CLI Agent") {
		fillMissingMetadata(db, i)
	}
	if i.Identifier != "" {
		tx, err := db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()

		if i.IsUser {
			var matchID int64
			err := tx.QueryRow("SELECT id FROM interactions WHERE session_id = ? AND is_user = 1 AND identifier = '' AND (message = ? OR message = ?) ORDER BY id DESC LIMIT 1", i.SessionID, i.Message, i.DetailedMessage).Scan(&matchID)
			if err == nil {
				_, err = tx.Exec("UPDATE interactions SET identifier = ? WHERE id = ?", i.Identifier, matchID)
				if err != nil {
					return err
				}
			}
		}

		// Check if it already exists
		var id int64
		var timestamp time.Time
		var existingTitle string
		var existingMessage string
		var existingDetailedMessage string
		var existingLink string
		var existingStatus string
		var existingKind string
		var existingAgent string
		var existingSessionID string
		var existingSessionPath string
		var existingIsUser bool
		err = tx.QueryRow("SELECT id, timestamp, title, message, detailed_message, link, status, kind, agent, session_id, session_path, is_user, quiet FROM interactions WHERE identifier = ? AND (session_id = ? OR ? = '')", i.Identifier, i.SessionID, i.SessionID).Scan(&id, &timestamp, &existingTitle, &existingMessage, &existingDetailedMessage, &existingLink, &existingStatus, &existingKind, &existingAgent, &existingSessionID, &existingSessionPath, &existingIsUser, &existingQuiet)
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
			if i.Kind == "" {
				i.Kind = existingKind
			}
			if i.Agent == "" {
				i.Agent = existingAgent
			}
			if i.SessionID == "" {
				i.SessionID = existingSessionID
			}
			if i.SessionPath == "" {
				i.SessionPath = existingSessionPath
			}
			i.IsUser = existingIsUser
			if !i.Replace {
				i.Message = mergeStrings(existingMessage, i.Message)

				if i.Kind == "approval" || existingKind == "approval" || strings.Contains(i.Title, "ToolPermission") {
					if i.DetailedMessage == "" {
						i.DetailedMessage = existingDetailedMessage
					}
				} else {
					i.DetailedMessage = mergeStrings(existingDetailedMessage, i.DetailedMessage)
				}
			}
			_, err = tx.Exec("UPDATE interactions SET title = ?, message = ?, detailed_message = ?, link = ?, is_user = ?, quiet = ?, status = ?, kind = ?, agent = ?, session_id = ?, session_path = ? WHERE id = ?", i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Kind, i.Agent, i.SessionID, i.SessionPath, id)
			if err != nil {
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}

			// If this was a "ready" status message, find the last "agent" message in the same session
			// and mark it as "done" if it's still "working".
			if i.Kind == "status" && i.Status == "r" && i.SessionID != "" {
				_, _ = db.Exec("UPDATE interactions SET status = 'd' WHERE session_id = ? AND kind = 'agent' AND status = 'w' AND id < ?", i.SessionID, id)
			}

			i.ID = id
			i.Timestamp = timestamp
			i.Update = true
		} else if err == sql.ErrNoRows {
			// Not found, insert new
			res, err := tx.Exec("INSERT INTO interactions (identifier, title, message, detailed_message, link, is_user, quiet, status, kind, agent, session_id, session_path) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", i.Identifier, i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Kind, i.Agent, i.SessionID, i.SessionPath)
			if err != nil {
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			id, _ := res.LastInsertId()

			// If this was a "ready" status message, find the last "agent" message in the same session
			// and mark it as "done" if it's still "working".
			if i.Kind == "status" && i.Status == "r" && i.SessionID != "" {
				_, _ = db.Exec("UPDATE interactions SET status = 'd' WHERE session_id = ? AND kind = 'agent' AND status = 'w' AND id < ?", i.SessionID, id)
			}

			i.ID = id
			i.Timestamp = time.Now().UTC()
		} else {
			return err
		}
	} else {

		res, err := db.Exec("INSERT INTO interactions (identifier, title, message, detailed_message, link, is_user, quiet, status, kind, agent, session_id, session_path) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", "", i.Title, i.Message, i.DetailedMessage, i.Link, i.IsUser, i.Quiet, i.Status, i.Kind, i.Agent, i.SessionID, i.SessionPath)
		if err != nil {
			return err
		}
		id, _ := res.LastInsertId()
		i.ID = id
		i.Timestamp = time.Now().UTC()
	}

	// Trigger Push for non-user messages only, and only if not quiet.
	// For updates, we only trigger if it's the first time it becomes non-quiet.
	if !i.IsUser && !i.Quiet {
		if !i.Update || existingQuiet {
			go sendPushNotifications(db, i.Title, i.Message, i.Link)
		}
	}

	// Handle global /new-agent command
	if i.IsUser && i.SessionID == "" && strings.HasPrefix(i.Message, "/new-agent") {
		go func() {
			args := strings.Fields(i.Message)
			subdir := "."
			if len(args) > 1 {
				subdir = args[1]
			}

			exe, err := os.Executable()
			if err != nil {
				log.Printf("Failed to get executable path: %v", err)
				return
			}
			exe, _ = filepath.Abs(exe)

			cwd, _ := os.Getwd()
			fullPath := filepath.Join(cwd, subdir)
			name := filepath.Base(fullPath)
			if subdir == "." || subdir == "" {
				name = "agent"
			}

			// We use tmux new-window to start the new agent.
			cmdStr := fmt.Sprintf("cd %s && %s --gemini-agent %s --yolo", fullPath, exe, name)
			cmd := exec.Command("tmux", "new-window", "-n", name, cmdStr)
			if err := cmd.Run(); err != nil {
				log.Printf("Failed to start new agent: %v (Command: %s)", err, cmdStr)
				// Broadcast a status message about the failure
				broadcaster.Broadcast(Interaction{
					Title:     "System",
					Message:   fmt.Sprintf("Failed to start new agent in %s: %v", subdir, err),
					Status:    "err",
					Agent:     "remote",
					Timestamp: time.Now().UTC(),
				})
			} else {
				// Broadcast a status message about the success to clear "working" state
				broadcaster.Broadcast(Interaction{
					Title:     "System",
					Message:   fmt.Sprintf("Started new agent in %s", subdir),
					Status:    "r",
					Agent:     "remote",
					Timestamp: time.Now().UTC(),
				})
			}
		}()
	}

	// Broadcast for streaming to all clients
	broadcaster.Broadcast(*i)
	return nil
}

func fillMissingMetadata(db *sql.DB, i *Interaction) {
	if i.SessionID == "" {
		return
	}
	var existingTitle string
	var existingAgent string
	var existingSessionPath string
	systemTitles := "'session-register', 'session-active', 'session-inactive', 'tmux-service', 'heartbeat'"
	// Try to find the most recent non-empty metadata for this session, excluding tmux status messages and system titles
	err := db.QueryRow(fmt.Sprintf("SELECT title, agent, session_path FROM interactions WHERE session_id = ? AND session_path != '' AND agent != 'tmux' AND title NOT IN (%s) ORDER BY id DESC LIMIT 1", systemTitles), i.SessionID).Scan(&existingTitle, &existingAgent, &existingSessionPath)
	if err == nil {
		if i.SessionPath == "" {
			i.SessionPath = existingSessionPath
		}
		if i.Agent == "" || i.Agent == "remote" {
			i.Agent = existingAgent
		}
		// Only inherit title if current is generic
		if i.Title == "" || i.Title == "Gemini" || i.Title == "Antigravity" || i.Title == "Remote" || i.Title == "CLI Agent" || i.Title == "Hermes Agent" || i.Title == "Claude" {
			i.Title = existingTitle
		}
	} else {
		// Try again without the session_path constraint if we still need title/agent
		_ = db.QueryRow(fmt.Sprintf("SELECT title, agent, session_path FROM interactions WHERE session_id = ? AND agent != 'tmux' AND title NOT IN (%s) AND (title != '' AND title != 'Gemini' AND title != 'Antigravity' AND title != 'Remote' AND title != 'CLI Agent' AND title != 'Hermes Agent' AND title != 'Claude') ORDER BY id DESC LIMIT 1", systemTitles), i.SessionID).Scan(&existingTitle, &existingAgent, &existingSessionPath)
		if existingTitle != "" && (i.Title == "" || i.Title == "Gemini" || i.Title == "Antigravity" || i.Title == "Remote" || i.Title == "CLI Agent" || i.Title == "Hermes Agent" || i.Title == "Claude") {
			i.Title = existingTitle
		}
		if existingAgent != "" && (i.Agent == "" || i.Agent == "remote") {
			i.Agent = existingAgent
		}
		if existingSessionPath != "" && i.SessionPath == "" {
			i.SessionPath = existingSessionPath
		}
	}
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

func runCliClient(ctx context.Context, address string, mode string, tmuxTarget string, sessionID string, sessionName string, sessionPath string, model string, yolo bool, stdin io.Reader, stdout io.Writer, stderr io.Writer) {
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
	if mode == "tmux" {
		agent = "tmux"
	}
	if model != "" {
		if strings.Contains(strings.ToLower(model), "gemini") {
			agent = "gemini"
		} else if strings.Contains(strings.ToLower(model), "antigravity") || strings.Contains(strings.ToLower(model), "agy") {
			agent = "antigravity"
		} else if strings.Contains(strings.ToLower(model), "claude") {
			agent = "claude"
		}
	}

	sendMsg := func(text string, title string, agent string, status string) {
		i := Interaction{Message: text, Title: title, SessionID: sessionID, SessionPath: sessionPath, Agent: agent, Status: status}
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
			exitMsg := fmt.Sprintf("[%s] No longer forwarding responses", title)
			if clientID != "" {
				exitMsg += fmt.Sprintf(" (Client ID: %s)", clientID)
			}
			sendMsg(exitMsg, title, agent, "")
			time.Sleep(100 * time.Millisecond) // Give the exit message a moment
		}()
	}

	// Registration
	regMsg := "Registered session: " + title
	if clientID != "" {
		regMsg += " (Client ID: " + clientID + ")"
	}
	sendMsg(regMsg, "session-register", agent, "r")

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
				sendMsg(fmt.Sprintf("Registered session: %s", title), "session-register", agent, "r")
			}
			if mode == "tmux" {
				msg := fmt.Sprintf("[%s] Now forwarding responses to %s", title, tmuxTarget)
				if clientID != "" {
					msg += fmt.Sprintf(" (Client ID: %s)", clientID)
				}
				sendMsg(msg, title, agent, "")
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
					if i.IsUser && i.Identifier == "" {
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

						if msg == "/restart" || msg == "/restart resume" {
							mode := "fresh"
							if msg == "/restart resume" {
								mode = "resume"
							}
							_ = os.WriteFile(".gemini-agent.restart", []byte(mode), 0644)
							msg = "/quit"
						}

						if msg == "/stop" && i.SessionID != "" && i.SessionID == sessionID {
							// Send Escape to tmux to interrupt the agent
							cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", tmuxTarget, "Escape")
							if err := cmd.Run(); err != nil {
								fmt.Fprintf(stderr, "\rFailed to send Escape to tmux: %v (Target: %s)\n", err, tmuxTarget)
							}
							// Send status update to clear "working" state in UI
							sendMsg("Stopped", title, agent, "r")
							continue
						}

						if strings.HasPrefix(msg, "/new-agent") && i.SessionID != "" && i.SessionID == sessionID {
							parts := strings.Fields(msg)
							target := ""
							if len(parts) > 1 {
								target = parts[1]
							}

							exe, _ := os.Executable()
							windowName := target
							if windowName == "" {
								windowName = filepath.Base(sessionPath)
							}
							windowName += "-agent"

							newPath := sessionPath
							if target != "" {
								p := target
								if !filepath.IsAbs(p) {
									p = filepath.Join(sessionPath, p)
								}
								if info, err := os.Stat(p); err == nil && info.IsDir() {
									newPath = p
								}
							}

							agentFlag := "--gemini-agent"
							if agent == "antigravity" {
								agentFlag = "--antigravity"
							}
							args := []string{"new-window", "-n", windowName, "-c", newPath, "--", exe, "--address", address, agentFlag}
							if target != "" {
								// If target was a path, use the base name for the agent name
								args = append(args, filepath.Base(target))
							}
							args = append(args, "--yolo")

							cmd := exec.Command("tmux", args...)
							if err := cmd.Run(); err != nil {
								fmt.Fprintf(stderr, "\rFailed to start new agent: %v\n", err)
							} else {
								// Send confirmation message to clear "working" status
								conf := Interaction{
									SessionID: sessionID,
									Title:     sessionName,
									Message:   fmt.Sprintf("Started new agent: %s", windowName),
									Status:    "r",
									Agent:     "remote",
								}
								data, _ := json.Marshal(conf)
								req, _ := http.NewRequestWithContext(ctx, "POST", fmt.Sprintf("http://%s/service?stream=false", address), bytes.NewReader(append(data, '\n')))
								req.Header.Set("Content-Type", "application/x-ndjson")
								resp, err := http.DefaultClient.Do(req)
								if err == nil {
									resp.Body.Close()
								}
							}
							continue
						}

						// Send the message
						cmd := exec.CommandContext(ctx, "tmux", "send-keys", "-t", tmuxTarget, "-l", "--", msg)
						if err := cmd.Run(); err != nil {
							fmt.Fprintf(stderr, "\rFailed to send keys to tmux: %v (Target: %s)\n", err, tmuxTarget)
						}
						// Small sleep before Enter
						time.Sleep(200 * time.Millisecond)
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

func runHermesAgent(ctx context.Context, hermesURL, pushAddress, sessionID, sessionName string) {
	if sessionID == "" {
		sessionID = "hermes-" + time.Now().Format("20060102-150405")
	}
	if sessionName == "" {
		sessionName = "Hermes Agent"
	}

	// Normalize Hermes URL: default to /v1/chat/completions if no path is provided
	if !strings.Contains(hermesURL, "/v1/") {
		hermesURL = strings.TrimSuffix(hermesURL, "/") + "/v1/chat/completions"
	}

	sendMsg := func(text string, title string, agent string, status string, identifier string, replace bool) {
		i := Interaction{
			Message:    text,
			Title:      title,
			SessionID:  sessionID,
			Agent:      agent,
			Status:     status,
			Identifier: identifier,
			Replace:    replace,
		}
		data, _ := json.Marshal(i)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Post(fmt.Sprintf("http://%s/service?stream=false", pushAddress), "application/x-ndjson", bytes.NewReader(append(data, '\n')))
		if err == nil {
			resp.Body.Close()
		}
	}

	// Initial registration
	sendMsg("Connected to Hermes Agent API Proxy (Standard OpenAI Mode)", "session-register", "hermes", "r", "", false)

	// Listen for user messages from Push to forward to Hermes
	var lastTimestamp time.Time
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		url := fmt.Sprintf("http://%s/service?session_id=%s", pushAddress, sessionID)
		if !lastTimestamp.IsZero() {
			url += "&timestamp=" + lastTimestamp.Format(time.RFC3339)
		}
		req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}

		dec := json.NewDecoder(resp.Body)
		for {
			var i Interaction
			if err := dec.Decode(&i); err != nil {
				resp.Body.Close()
				break
			}
			if i.ID == 0 {
				continue
			}
			if i.Timestamp.After(lastTimestamp) {
				lastTimestamp = i.Timestamp
			}

			if i.IsUser && i.Message != "" {
				// Forward to Hermes using standard OpenAI streaming pattern
				go func(userMsg Interaction) {
					// 1. Prepare OpenAI-compatible request
					payload := map[string]interface{}{
						"model":  "hermes-agent",
						"stream": true,
						"messages": []map[string]string{
							{"role": "user", "content": userMsg.Message},
						},
					}
					jsonData, _ := json.Marshal(payload)
					
					hReq, err := http.NewRequestWithContext(ctx, "POST", hermesURL, bytes.NewReader(jsonData))
					if err != nil {
						sendMsg("Error creating Hermes request: "+err.Error(), sessionName, "hermes", "err", "", false)
						return
					}
					hReq.Header.Set("Content-Type", "application/json")
					hReq.Header.Set("Accept", "text/event-stream")

					hResp, err := http.DefaultClient.Do(hReq)
					if err != nil {
						sendMsg("Error connecting to Hermes: "+err.Error(), sessionName, "hermes", "err", "", false)
						return
					}
					defer hResp.Body.Close()

					if hResp.StatusCode != http.StatusOK {
						body, _ := io.ReadAll(hResp.Body)
						sendMsg(fmt.Sprintf("Hermes API error (%d): %s", hResp.StatusCode, string(body)), sessionName, "hermes", "err", "", false)
						return
					}

					// 2. Parse SSE response from the POST request
					scanner := bufio.NewScanner(hResp.Body)
					var currentID string
					var currentMsg string
					var eventType string

					for scanner.Scan() {
						line := scanner.Text()
						if line == "" {
							eventType = ""
							continue
						}

						if strings.HasPrefix(line, "event: ") {
							eventType = strings.TrimPrefix(line, "event: ")
							continue
						}

						if strings.HasPrefix(line, "data: ") {
							data := strings.TrimPrefix(line, "data: ")
							if data == "[DONE]" {
								if currentID != "" {
									sendMsg(currentMsg, sessionName, "hermes", "r", currentID, true)
								}
								currentID = ""
								currentMsg = ""
								continue
							}

							if eventType == "hermes.tool.progress" {
								var progress struct {
									Tool   string `json:"tool"`
									Input  string `json:"input"`
									Status string `json:"status"`
								}
								if err := json.Unmarshal([]byte(data), &progress); err == nil {
									msg := fmt.Sprintf("🔧 **%s**: `%s` (%s)", progress.Tool, progress.Input, progress.Status)
									sendMsg(msg, sessionName, "hermes", "w", "", false)
								}
								continue
							}

							// Try to parse as OpenAI-compatible chunk
							var chunk struct {
								Choices []struct {
									Delta struct {
										Content string `json:"content"`
									} `json:"delta"`
								} `json:"choices"`
							}

							if err := json.Unmarshal([]byte(data), &chunk); err == nil && len(chunk.Choices) > 0 {
								if chunk.Choices[0].Delta.Content != "" {
									if currentID == "" {
										currentID = fmt.Sprintf("hermes-sse-%d", time.Now().UnixNano())
									}
									currentMsg += chunk.Choices[0].Delta.Content
									sendMsg(currentMsg, sessionName, "hermes", "w", currentID, true)
								}
							}
						}
					}
				}(i)
			}
		}
		time.Sleep(1 * time.Second)
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
				// Broadcast with metadata if available
				i := Interaction{
					Title:     "session-active",
					SessionID: sessionID,
				}
				fillMissingMetadata(db, &i)
				go broadcaster.Broadcast(i)
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
		var startID int64
		if afterStr := r.URL.Query().Get("after"); afterStr != "" {
			fmt.Sscanf(afterStr, "%d", &startID)
		} else if tsStr := r.URL.Query().Get("timestamp"); tsStr != "" {
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
		query := "SELECT id, identifier, title, message, detailed_message, link, is_user, quiet, timestamp, status, kind, agent, session_id, session_path FROM interactions WHERE "
		var args []interface{}
		if startID > 0 {
			query += "id > ?"
			args = append(args, startID)
		} else {
			query += "timestamp > ?"
			args = append(args, startTime)
		}
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
				if err := rows.Scan(&i.ID, &i.Identifier, &i.Title, &i.Message, &i.DetailedMessage, &i.Link, &i.IsUser, &i.Quiet, &i.Timestamp, &i.Status, &i.Kind, &i.Agent, &i.SessionID, &i.SessionPath); err == nil {
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

func handleRenameSession(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		oldID := r.URL.Query().Get("old")
		newID := r.URL.Query().Get("new")
		if oldID == "" || newID == "" {
			http.Error(w, "Missing old or new session ID", http.StatusBadRequest)
			return
		}
		_, err := db.Exec("UPDATE interactions SET session_id = ? WHERE session_id = ?", newID, oldID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		sessionsMu.Lock()
		if count, exists := activeSessions[oldID]; exists {
			activeSessions[newID] += count
			delete(activeSessions, oldID)
		}
		sessionsMu.Unlock()

		broadcaster.Broadcast(Interaction{
			Title:     "session-rename",
			Message:   oldID,
			SessionID: newID,
		})

		w.WriteHeader(http.StatusOK)
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

func runGeminiAgent(args []string, address string) {
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error getting executable path: %v\n", err)
		os.Exit(1)
	}

	// Create a temporary file for the script to allow stdin pass-through
	tmpFile, err := os.CreateTemp("", "gemini-agent-*.sh")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating temporary script file: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(tmpFile.Name())

	if _, err := tmpFile.WriteString(geminiAgentScript); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing temporary script file: %v\n", err)
		os.Exit(1)
	}
	tmpFile.Close()

	cmd := exec.Command("bash", append([]string{tmpFile.Name()}, args...)...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = append(os.Environ(), "PUSH_BINARY="+exe, "PUSH_ADDRESS="+address)

	// Handle signals to ensure we wait for the child process to exit and clean up
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	if err := cmd.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting gemini-agent: %v\n", err)
		os.Exit(1)
	}

	// Wait for the command to finish in a goroutine so we can still catch signals
	done := make(chan error, 1)
	go func() {
		done <- cmd.Wait()
	}()

	select {
	case sig := <-sigChan:
		// Forward the signal to the child process (though it likely already received it if in the same PGID)
		_ = cmd.Process.Signal(sig)
		// Wait for the process to actually exit
		err = <-done
	case err = <-done:
		// Process exited on its own
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "Error running gemini-agent: %v\n", err)
		os.Exit(1)
	}
}

func runAgyScraper(logDir, logFile, backendURL, fallbackSessionID, sessionPath string, yolo bool) {
	if logFile != "" {
		fmt.Fprintf(os.Stderr, "Watching log file: %s\n", logFile)
	} else {
		fmt.Fprintf(os.Stderr, "Watching log directory: %s\n", logDir)
	}

	var currentLogFile string
	var fileHandle *os.File
	var reader *bufio.Reader
	var lineAccumulator strings.Builder

	seenMessages := make(map[string]string) // Map id -> last content
	sessionID := fallbackSessionID
	lastProcessedMsgFinalized := false
	lastApprovalID := ""

	getLatestLogFile := func(dir string) string {
		if logFile != "" {
			if _, err := os.Stat(logFile); err == nil {
				return logFile
			}
			return ""
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return ""
		}
		var latest string
		var latestTime time.Time
		for _, entry := range entries {
			if !entry.IsDir() && strings.HasPrefix(entry.Name(), "session-") && strings.HasSuffix(entry.Name(), ".jsonl") {
				info, err := entry.Info()
				if err != nil {
					continue
				}
				if info.ModTime().After(latestTime) {
					latestTime = info.ModTime()
					latest = filepath.Join(dir, entry.Name())
				}
			}
		}
		return latest
	}

	send := func(payload Interaction) {
		data, _ := json.Marshal(payload)
		resp, err := http.Post(backendURL+"/interactions", "application/json", bytes.NewReader(data))
		if err == nil {
			resp.Body.Close()
		}
	}

	for {
		latest := getLatestLogFile(logDir)
		if latest != "" && latest != currentLogFile {
			if fileHandle != nil {
				fileHandle.Close()
			}

			info, _ := os.Stat(latest)
			startAtEnd := currentLogFile == "" && time.Since(info.ModTime()) > 2*time.Minute && logFile == ""

			if currentLogFile == "" {
				fmt.Fprintf(os.Stderr, "Found log file: %s (startAtEnd=%v)\n", latest, startAtEnd)
			} else {
				fmt.Fprintf(os.Stderr, "Switching to log file: %s\n", latest)
			}
			currentLogFile = latest
			h, err := os.Open(currentLogFile)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}
			fileHandle = h
			reader = bufio.NewReader(fileHandle)
			lineAccumulator.Reset()

			if startAtEnd {
				fileHandle.Seek(0, io.SeekEnd)
			}

			seenMessages = make(map[string]string)
			lastProcessedMsgFinalized = false
		}

		if reader == nil {
			time.Sleep(1 * time.Second)
			continue
		}

		line, err := reader.ReadString('\n')
		if line != "" {
			lineAccumulator.WriteString(line)
			if strings.HasSuffix(line, "\n") {
				fullLine := strings.TrimSpace(lineAccumulator.String())
				lineAccumulator.Reset()
				if fullLine == "" {
					continue
				}

				var data AgyLogLine
				if err := json.Unmarshal([]byte(fullLine), &data); err != nil {
					continue
				}

				if data.Set != nil {
					if lastProcessedMsgFinalized {
						payload := Interaction{
							SessionID:   sessionID,
							SessionPath: sessionPath,
							Status:      "r",
							Kind:        "status",
							Message:     "Ready",
							Quiet:       true,
						}
						send(payload)
						lastProcessedMsgFinalized = false
					}
					continue
				}

				if data.StepIndex != nil && data.ID == "" {
					data.ID = fmt.Sprintf("%d", *data.StepIndex)
				}

				if data.ID == "" {
					continue
				}

				if lastApprovalID != "" && lastApprovalID != data.ID+"-approval" && lastApprovalID != data.ID+"-question" {
					// Mark previous card as done/resolved
					resolvedKind := "approval"
					if strings.HasSuffix(lastApprovalID, "-question") {
						resolvedKind = "question"
					}
					payload := Interaction{
						Identifier:  lastApprovalID,
						Status:      "d",
						Kind:        resolvedKind,
						Agent:       "antigravity",
						SessionID:   sessionID,
						SessionPath: sessionPath,
						Quiet:       true,
					}
					send(payload)
					lastApprovalID = ""
				}

				if data.SessionID != "" {
					sessionID = data.SessionID
				}

				isFinalized := (data.Tokens != nil && data.Tokens["total"] > 0) || data.Status == "DONE" || data.Status == "ERROR"
				thoughtText := data.Thinking
				if len(data.Thoughts) > 0 {
					var thoughts []string
					for _, t := range data.Thoughts {
						thoughts = append(thoughts, fmt.Sprintf("**%s**: %s", t.Subject, t.Description))
					}
					thoughtText = strings.Join(thoughts, "\n\n")
				}

				status := "w"
				isUser := false
				kind := "status"

				if data.Source == "MODEL" {
					if data.Type == "PLANNER_RESPONSE" {
						kind = "agent"
						isUser = false
						if isFinalized && len(data.ToolCalls) == 0 {
							status = "d"
							go func() {
								time.Sleep(50 * time.Millisecond)
								send(Interaction{
									SessionID:   sessionID,
									SessionPath: sessionPath,
									Status:      "r",
									Kind:        "status",
									Message:     "Ready",
									Quiet:       true,
								})
							}()
						} else {
							status = "w"
						}
					} else if data.Type == "USER_INPUT" || data.Type == "CONVERSATION_HISTORY" {
						continue // Ignore these or handle them as system
					} else {
						// Other model/system steps (like tool outputs RUN_COMMAND, VIEW_FILE, etc.)
						kind = "tool"
						status = "d" // The tool run itself is complete
					}
				} else if data.Source == "USER_EXPLICIT" {
					kind = "status"
					isUser = true
					status = "d"
					isFinalized = true
					// Extract content between <USER_REQUEST> tags
					startTag := "<USER_REQUEST>"
					endTag := "</USER_REQUEST>"
					startIdx := strings.Index(data.Content, startTag)
					if startIdx != -1 {
						startIdx += len(startTag)
						endIdx := strings.Index(data.Content[startIdx:], endTag)
						if endIdx != -1 {
							data.Content = strings.TrimSpace(data.Content[startIdx : startIdx+endIdx])
						} else {
							data.Content = strings.TrimSpace(data.Content[startIdx:])
						}
					}
				} else {
					continue // Ignore System and others
				}

				content := data.Content
				if content == "" {
					content = "Working..."
				}
				shortMsg := content
				if kind != "tool" && len(shortMsg) > 100 {
					shortMsg = shortMsg[:100]
				}

				detailed := content
				if thoughtText != "" {
					detailed += "\n\n### Thoughts\n" + thoughtText
				}

				payload := Interaction{
					Identifier:      data.ID,
					Message:         shortMsg,
					DetailedMessage: detailed,
					Title:           filepath.Base(sessionPath),
					Agent:           "antigravity",
					Kind:            kind,
					Status:          status,
					SessionID:       sessionID,
					SessionPath:     sessionPath,
					IsUser:          isUser,
					Quiet:           true,
				}

				if seenMessages[data.ID] != fullLine {
					seenMessages[data.ID] = fullLine
					send(payload)

					// Check for special tool calls like ask_question, or generate approval card (skip approval in YOLO mode)
					if data.Source == "MODEL" && data.Type == "PLANNER_RESPONSE" && len(data.ToolCalls) > 0 {
						firstTool := data.ToolCalls[0]
						if firstTool.Name == "ask_question" {
							type QuestionParam struct {
								Question      string   `json:"question"`
								Options       []string `json:"options"`
								IsMultiSelect bool     `json:"is_multi_select"`
							}
							type AskQuestionArgs struct {
								Questions []QuestionParam `json:"questions"`
							}
							var args AskQuestionArgs
							if err := json.Unmarshal(firstTool.Args, &args); err == nil {
								type UIOption struct {
									Label string `json:"label"`
								}
								type UIQuestion struct {
									Header   string     `json:"header"`
									Question string     `json:"question"`
									Type     string     `json:"type"`
									Options  []UIOption `json:"options"`
								}
								type UIQuestionPayload struct {
									Questions []UIQuestion `json:"questions"`
								}
								var uiPayload UIQuestionPayload
								for _, q := range args.Questions {
									qType := "text"
									var uiOptions []UIOption
									if len(q.Options) > 0 {
										qType = "choice"
										for _, opt := range q.Options {
											uiOptions = append(uiOptions, UIOption{Label: opt})
										}
									}
									uiPayload.Questions = append(uiPayload.Questions, UIQuestion{
										Header:   "Question",
										Question: q.Question,
										Type:     qType,
										Options:  uiOptions,
									})
								}
								payloadJSON, _ := json.Marshal(uiPayload)

								questionPayload := Interaction{
									Identifier:      data.ID + "-question",
									Message:         "Information requested",
									DetailedMessage: string(payloadJSON),
									Title:           "Question",
									Agent:           "antigravity",
									Kind:            "question",
									Status:          "awaiting",
									SessionID:       sessionID,
									SessionPath:     sessionPath,
									Quiet:           false, // Always show questions
								}
								send(questionPayload)
								lastApprovalID = data.ID + "-question"
							}
						} else if !yolo {
							// Create approval details JSON
							details := map[string]interface{}{
								"tool_name":  firstTool.Name,
								"tool_input": firstTool.Args,
							}
							// If the tool is a command execution, map to standard type/command format
							if firstTool.Name == "run_command" {
								var argsMap map[string]interface{}
								if err := json.Unmarshal(firstTool.Args, &argsMap); err == nil {
									if cmdVal, ok := argsMap["CommandLine"]; ok {
										details["type"] = "exec"
										details["command"] = cmdVal
									}
								}
							} else if firstTool.Name == "write_to_file" || firstTool.Name == "replace_file_content" || firstTool.Name == "multi_replace_file_content" {
								var argsMap map[string]interface{}
								if err := json.Unmarshal(firstTool.Args, &argsMap); err == nil {
									if fileVal, ok := argsMap["TargetFile"]; ok {
										details["type"] = "edit"
										details["fileName"] = fileVal
									}
								}
							}
							detailsJSON, _ := json.Marshal(details)

							approvalPayload := Interaction{
								Identifier:      data.ID + "-approval",
								Message:         "Approval requested",
								DetailedMessage: string(detailsJSON),
								Title:           "ToolPermission",
								Agent:           "antigravity",
								Kind:            "approval",
								Status:          "awaiting",
								SessionID:       sessionID,
								SessionPath:     sessionPath,
								Quiet:           yolo, // Quiet in yolo mode
							}
							send(approvalPayload)
							lastApprovalID = data.ID + "-approval"
						}
					}
				}
				lastProcessedMsgFinalized = isFinalized
			}
		}

		if err != nil {
			if err == io.EOF {
				// Check for truncation
				if info, err := os.Stat(currentLogFile); err == nil {
					pos, _ := fileHandle.Seek(0, io.SeekCurrent)
					if info.Size() < pos {
						fmt.Fprintf(os.Stderr, "File truncated, resetting: %s\n", currentLogFile)
						fileHandle.Seek(0, io.SeekStart)
						reader = bufio.NewReader(fileHandle)
						lineAccumulator.Reset()
					}
				}
				time.Sleep(100 * time.Millisecond)
				continue
			}
			// Other error, log and wait
			fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
			time.Sleep(1 * time.Second)
		}
	}
}

func translateAgentArgs(isAntigravity, resume, yolo bool, extraArgs []string) []string {
	var agentArgs []string
	if isAntigravity {
		agentArgs = append(agentArgs, "--agent", "agy")
	}
	if resume {
		if isAntigravity {
			agentArgs = append(agentArgs, "--continue")
		} else {
			agentArgs = append(agentArgs, "--resume")
		}
	}
	if yolo {
		agentArgs = append(agentArgs, "--yolo")
	}
	for _, arg := range extraArgs {
		if isAntigravity && (arg == "--resume" || arg == "-resume" || arg == "—resume") {
			agentArgs = append(agentArgs, "--continue")
		} else if !isAntigravity && (arg == "--continue" || arg == "-continue" || arg == "—continue") {
			agentArgs = append(agentArgs, "--resume")
		} else {
			agentArgs = append(agentArgs, arg)
		}
	}
	return agentArgs
}

func normalizeArgs(args []string) []string {
	normalized := make([]string, len(args))
	copy(normalized, args)
	for i, arg := range normalized {
		if i == 0 {
			continue // Skip executable path
		}
		if strings.HasPrefix(arg, "—") { // em-dash (U+2014)
			normalized[i] = "--" + strings.TrimPrefix(arg, "—")
		} else if strings.HasPrefix(arg, "–") { // en-dash (U+2013)
			normalized[i] = "--" + strings.TrimPrefix(arg, "–")
		}
	}
	return normalized
}



