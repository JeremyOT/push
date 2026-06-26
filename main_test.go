package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

func TestBroadcaster(t *testing.T) {
	b := &Broadcaster{
		subscribers: make(map[chan Interaction]bool),
	}

	ch1 := b.Subscribe()
	ch2 := b.Subscribe()

	interaction := Interaction{
		ID:      1,
		Title:   "Test Title",
		Message: "Test Message",
	}

	b.Broadcast(interaction)

	select {
	case received := <-ch1:
		if received.ID != interaction.ID {
			t.Errorf("Expected ID %d, got %d", interaction.ID, received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch1 did not receive interaction")
	}

	select {
	case received := <-ch2:
		if received.ID != interaction.ID {
			t.Errorf("Expected ID %d, got %d", interaction.ID, received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 did not receive interaction")
	}

	b.Unsubscribe(ch1)
	b.Broadcast(interaction)

	select {
	case _, ok := <-ch1:
		if ok {
			t.Error("ch1 should be closed after Unsubscribe")
		}
	case <-time.After(100 * time.Millisecond):
		// This is actually what we expect if it was closed and we drained it, 
		// but since we didn't drain it, it should be closed.
		// Wait, Unsubscribe closes the channel.
	}

	select {
	case received := <-ch2:
		if received.ID != interaction.ID {
			t.Errorf("Expected ID %d, got %d", interaction.ID, received.ID)
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("ch2 did not receive interaction")
	}
}

func TestGetStaticContent(t *testing.T) {
	staticRoot, _ := fs.Sub(staticFS, "static")

	// Test default index.html
	data, contentType, _, err := getStaticContent(staticRoot, "/", "", false, false)
	if err != nil {
		t.Fatalf("Failed to get index.html: %v", err)
	}
	if contentType != "text/html; charset=utf-8" {
		t.Errorf("Expected content type 'text/html; charset=utf-8', got '%s'", contentType)
	}
	if !strings.Contains(string(data), "<title>Push</title>") {
		t.Error("index.html did not contain default title")
	}

	// Test custom title
	data, _, _, _ = getStaticContent(staticRoot, "/", "Custom App", false, false)
	if !strings.Contains(string(data), "<title>Custom App</title>") {
		t.Error("index.html did not contain custom title")
	}

	// Test custom icons in index.html
	data, _, _, _ = getStaticContent(staticRoot, "/", "", true, false)
	if !strings.Contains(string(data), "icon.png") || !strings.Contains(string(data), "type=\"image/png\"") {
		t.Error("index.html did not contain custom icon references")
	}

	// Test interactive mode
	data, _, _, _ = getStaticContent(staticRoot, "/", "", false, true)
	if !strings.Contains(string(data), `{"interactive": true}`) {
		t.Error("index.html did not contain interactive: true")
	}

	// Test manifest.json custom title
	data, _, _, _ = getStaticContent(staticRoot, "/manifest.json", "Custom App", false, false)
	if !strings.Contains(string(data), `"name": "Custom App"`) {
		t.Error("manifest.json did not contain custom title")
	}

	// Test sw.js custom title
	data, _, _, _ = getStaticContent(staticRoot, "/sw.js", "Custom App", false, false)
	if !strings.Contains(string(data), "let title = 'Custom App';") {
		t.Error("sw.js did not contain custom title")
	}

	// Test not found
	_, _, _, err = getStaticContent(staticRoot, "/nonexistent", "", false, false)
	if err == nil {
		t.Error("Expected error for nonexistent file, got nil")
	}
}

func TestResizeImage(t *testing.T) {
	// Create a simple 10x10 red square
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			img.Set(x, y, image.Rect(0, 0, 0, 0).At(0, 0)) // transparent
		}
	}

	data, err := resizeImage(img, 20)
	if err != nil {
		t.Fatalf("Failed to resize image: %v", err)
	}

	decoded, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("Failed to decode resized image: %v", err)
	}

	if decoded.Bounds().Dx() != 20 || decoded.Bounds().Dy() != 20 {
		t.Errorf("Expected 20x20 image, got %dx%d", decoded.Bounds().Dx(), decoded.Bounds().Dy())
	}
}

func setupTestDB(t *testing.T) (*sql.DB, string) {
	tempDir, err := os.MkdirTemp("", "push-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	dbPath := filepath.Join(tempDir, "test.sqlite")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open test database: %v", err)
	}
	if err := initDB(db); err != nil {
		t.Fatalf("Failed to init test database: %v", err)
	}
	return db, tempDir
}

func TestDB(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Check if tables exist
	var name string
	err := db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='interactions'").Scan(&name)
	if err != nil {
		t.Errorf("interactions table not found: %v", err)
	}

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='subscriptions'").Scan(&name)
	if err != nil {
		t.Errorf("subscriptions table not found: %v", err)
	}

	err = db.QueryRow("SELECT name FROM sqlite_master WHERE type='table' AND name='config'").Scan(&name)
	if err != nil {
		t.Errorf("config table not found: %v", err)
	}
}

func TestInitVAPID(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Initial generation
	err := initVAPID(db)
	if err != nil {
		t.Fatalf("Failed to init VAPID: %v", err)
	}
	key1 := vapidPublicKey
	priv1 := vapidPrivateKey

	if key1 == "" || priv1 == "" {
		t.Error("VAPID keys were not generated")
	}

	// Loading existing
	vapidPublicKey = ""
	vapidPrivateKey = ""
	err = initVAPID(db)
	if err != nil {
		t.Fatalf("Failed to reload VAPID: %v", err)
	}
	if vapidPublicKey != key1 || vapidPrivateKey != priv1 {
		t.Errorf("VAPID keys were not reloaded correctly: got %s, expected %s", vapidPublicKey, key1)
	}
}

func TestHandleInteractions(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleInteractions(db)

	// Test POST
	interaction := Interaction{
		Title:   "Test POST",
		Message: "Hello World",
	}
	body, _ := json.Marshal(interaction)
	req := httptest.NewRequest("POST", "/interactions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var saved Interaction
	if err := json.NewDecoder(w.Body).Decode(&saved); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if saved.Message != "Hello World" {
		t.Errorf("Expected message 'Hello World', got '%s'", saved.Message)
	}
	if saved.ID == 0 {
		t.Error("Expected non-zero ID")
	}

	// Test POST with SessionID
	interactionSession := Interaction{
		Message:   "Session msg",
		SessionID: "sess-123",
	}
	bodySess, _ := json.Marshal(interactionSession)
	reqSess := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodySess))
	wSess := httptest.NewRecorder()
	handler(wSess, reqSess)

	var savedSess Interaction
	json.Unmarshal(wSess.Body.Bytes(), &savedSess)
	if savedSess.SessionID != "sess-123" {
		t.Errorf("Expected session_id sess-123, got %s", savedSess.SessionID)
	}

	// Test POST with Identifier (Insert)
	interactionID := Interaction{
		Identifier: "task-1",
		Title:      "Task 1",
		Message:    "Started",
	}
	bodyID, _ := json.Marshal(interactionID)
	reqID := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyID))
	wID := httptest.NewRecorder()
	handler(wID, reqID)

	var savedID Interaction
	json.Unmarshal(wID.Body.Bytes(), &savedID)
	if savedID.Identifier != "task-1" || savedID.Message != "Started" || savedID.Update {
		t.Errorf("Unexpected saved interaction: %+v", savedID)
	}

	// Test POST with same Identifier (Default: Append)
	interactionAppend := Interaction{
		Identifier: "task-1",
		Title:      "Task 1",
		Message:    " (nearly done)",
	}
	bodyAppend, _ := json.Marshal(interactionAppend)
	reqAppend := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyAppend))
	wAppend := httptest.NewRecorder()
	handler(wAppend, reqAppend)

	var savedAppend Interaction
	json.Unmarshal(wAppend.Body.Bytes(), &savedAppend)
	if savedAppend.Message != "Started (nearly done)" || !savedAppend.Update {
		t.Errorf("Expected append, got: '%s' (update=%v)", savedAppend.Message, savedAppend.Update)
	}

	// Test POST with same Identifier (Explicit: Replace)
	interactionReplace := Interaction{
		Identifier: "task-1",
		Title:      "Task 1",
		Message:    "Completed",
		Replace:    true,
	}
	bodyReplace, _ := json.Marshal(interactionReplace)
	reqReplace := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyReplace))
	wReplace := httptest.NewRecorder()
	handler(wReplace, reqReplace)

	var savedReplace Interaction
	json.Unmarshal(wReplace.Body.Bytes(), &savedReplace)
	if savedReplace.ID != savedID.ID || savedReplace.Message != "Completed" || !savedReplace.Update {
		t.Errorf("Expected replace, got: %+v", savedReplace)
	}

	// Verify in GET
	reqGET := httptest.NewRequest("GET", "/interactions", nil)
	wGET := httptest.NewRecorder()
	handler(wGET, reqGET)
	var interactions []Interaction
	json.Unmarshal(wGET.Body.Bytes(), &interactions)
	
	found := false
	for _, it := range interactions {
		if it.Identifier == "task-1" {
			found = true
			if it.Message != "Completed" {
				t.Errorf("Expected 'Completed', got '%s'", it.Message)
			}
		}
	}
	if !found {
		t.Error("Did not find interaction with identifier 'task-1'")
	}

	// Test POST with Quiet
	interactionQuiet := Interaction{
		Title:   "Quiet POST",
		Message: "Shhh",
		Quiet:   true,
	}
	bodyQuiet, _ := json.Marshal(interactionQuiet)
	reqQuiet := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyQuiet))
	wQuiet := httptest.NewRecorder()
	handler(wQuiet, reqQuiet)

	if wQuiet.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", wQuiet.Code)
	}

	var savedQuiet Interaction
	json.NewDecoder(wQuiet.Body).Decode(&savedQuiet)
	if !savedQuiet.Quiet {
		t.Error("Expected quiet to be true in response")
	}

	// Test POST with Status and Agent
	interactionStatus := Interaction{
		Agent:   "gemini",
		Status:  "w",
		Message: "Thinking...",
	}
	bodyStatus, _ := json.Marshal(interactionStatus)
	reqStatus := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyStatus))
	wStatus := httptest.NewRecorder()
	handler(wStatus, reqStatus)

	var savedStatus Interaction
	json.Unmarshal(wStatus.Body.Bytes(), &savedStatus)
	if savedStatus.Agent != "gemini" || savedStatus.Status != "w" {
		t.Errorf("Expected agent=gemini, status=w, got: agent=%s, status=%s", savedStatus.Agent, savedStatus.Status)
	}

	// Verify in GET
	reqGETStatus := httptest.NewRequest("GET", "/interactions", nil)
	wGETStatus := httptest.NewRecorder()
	handler(wGETStatus, reqGETStatus)
	var interactionsStatus []Interaction
	json.Unmarshal(wGETStatus.Body.Bytes(), &interactionsStatus)

	foundStatus := false
	for _, it := range interactionsStatus {
		if it.ID == savedStatus.ID {
			foundStatus = true
			if it.Agent != "gemini" || it.Status != "w" {
				t.Errorf("GET: Expected agent=gemini, status=w, got: agent=%s, status=%s", it.Agent, it.Status)
			}
		}
	}
	if !foundStatus {
		t.Error("Did not find interaction with status and agent")
	}

	// Test POST with same Identifier (Merge: preserve title/agent/status if empty)
	interactionMerge := Interaction{
		Identifier: "task-1",
		Message:    " (verified)",
	}
	bodyMerge, _ := json.Marshal(interactionMerge)
	reqMerge := httptest.NewRequest("POST", "/interactions", bytes.NewReader(bodyMerge))
	wMerge := httptest.NewRecorder()
	handler(wMerge, reqMerge)

	var savedMerge Interaction
	json.Unmarshal(wMerge.Body.Bytes(), &savedMerge)
	if savedMerge.Title != "Task 1" || !strings.Contains(savedMerge.Message, "verified") {
		t.Errorf("Expected title 'Task 1' to be preserved, got '%s'. Message: '%s'", savedMerge.Title, savedMerge.Message)
	}

	// Test GET
	req = httptest.NewRequest("GET", "/interactions", nil)
	w = httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var list []Interaction
	if err := json.NewDecoder(w.Body).Decode(&list); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}
	if len(list) != 5 {
		t.Errorf("Expected 5 interactions, got %d", len(list))
	}
	if !list[3].Quiet {
		t.Errorf("Expected interaction at index 3 to have quiet=true, got quiet=%v (message: %s)", list[3].Quiet, list[3].Message)
	}
}

func TestHandleSubscribe(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleSubscribe(db)

	sub := map[string]interface{}{
		"endpoint": "https://example.com/push",
		"keys": map[string]string{
			"p256dh": "p256dh-key",
			"auth":   "auth-secret",
		},
	}
	body, _ := json.Marshal(sub)
	req := httptest.NewRequest("POST", "/subscribe", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	// Verify in DB
	var endpoint string
	err := db.QueryRow("SELECT endpoint FROM subscriptions WHERE endpoint = ?", "https://example.com/push").Scan(&endpoint)
	if err != nil {
		t.Errorf("Subscription not found in DB: %v", err)
	}
}

func TestHandleInteractionsPagination(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Insert 10 interactions
	for i := 1; i <= 10; i++ {
		db.Exec("INSERT INTO interactions (identifier, message) VALUES (?, ?)", "", fmt.Sprintf("Message %d", i))
	}

	handler := handleInteractions(db)

	// Test GET with limit
	req := httptest.NewRequest("GET", "/interactions?limit=5", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	var list []Interaction
	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 5 {
		t.Errorf("Expected 5 interactions, got %d", len(list))
	}
	// Initial load should be latest 5, reversed to ASC
	if list[0].Message != "Message 6" {
		t.Errorf("Expected Message 6, got %s", list[0].Message)
	}

	// Test GET with after
	req = httptest.NewRequest("GET", "/interactions?after=8", nil)
	w = httptest.NewRecorder()
	handler(w, req)

	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 2 {
		t.Errorf("Expected 2 interactions (9, 10), got %d", len(list))
	}
	if list[0].Message != "Message 9" {
		t.Errorf("Expected Message 9, got %s", list[0].Message)
	}

	// Test GET with before
	req = httptest.NewRequest("GET", "/interactions?before=4&limit=2", nil)
	w = httptest.NewRecorder()
	handler(w, req)

	json.NewDecoder(w.Body).Decode(&list)
	if len(list) != 2 {
		t.Errorf("Expected 2 interactions (2, 3), got %d", len(list))
	}
	if list[0].Message != "Message 2" {
		t.Errorf("Expected Message 2, got %s", list[0].Message)
	}
}

func TestHandleService(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleService(db)

	// Test POST to /service?stream=false
	interaction := Interaction{Message: "Service Message"}
	body, _ := json.Marshal(interaction)
	req := httptest.NewRequest("POST", "/service?stream=false", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected status 204, got %d", w.Code)
	}

	// Verify in DB
	var msg string
	db.QueryRow("SELECT message FROM interactions").Scan(&msg)
	if msg != "Service Message" {
		t.Errorf("Expected 'Service Message', got '%s'", msg)
	}

	// Test streaming (GET)
	req = httptest.NewRequest("GET", "/service", nil)
	ctx, cancel := context.WithCancel(req.Context())
	req = req.WithContext(ctx)
	
	w_stream := &streamRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		data:             make(chan string, 10),
	}

	go func() {
		handler(w_stream, req)
	}()

	// Wait for connection to be established (and maybe a heartbeat or initial data)
	// Broadcaster should send the new interaction
	time.Sleep(100 * time.Millisecond)
	
	newInteraction := Interaction{ID: 99, Message: "Stream me"}
	broadcaster.Broadcast(newInteraction)

	select {
	case line := <-w_stream.data:
		var received Interaction
		if err := json.Unmarshal([]byte(line), &received); err != nil {
			t.Fatalf("Failed to unmarshal streamed interaction: %v", err)
		}
		// If it's a heartbeat, read the next one
		if received.Title == "heartbeat" {
			select {
			case line := <-w_stream.data:
				if err := json.Unmarshal([]byte(line), &received); err != nil {
					t.Fatalf("Failed to unmarshal streamed interaction: %v", err)
				}
			case <-time.After(500 * time.Millisecond):
				t.Error("Did not receive any streamed interaction after heartbeat")
			}
		}

		// It might be the first message (Service Message) or the "Stream me" one.
		if received.Message != "Service Message" && received.Message != "Stream me" {
			t.Errorf("Unexpected message: %s (Title: %s)", received.Message, received.Title)
		}
	case <-time.After(500 * time.Millisecond):
		t.Error("Did not receive any streamed interaction")
	}
	
	cancel()
}

func TestSendMessage(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	server := httptest.NewServer(handleInteractions(db))
	defer server.Close()

	// server.Listener.Addr().String() will be something like "127.0.0.1:12345"
	addr := server.Listener.Addr().String()

	err := sendMessage(addr, "Test CLI Message", "CLI Title")
	if err != nil {
		t.Fatalf("sendMessage failed: %v", err)
	}

	// Verify in DB
	var msg, title string
	err = db.QueryRow("SELECT message, title FROM interactions").Scan(&msg, &title)
	if err != nil {
		t.Fatalf("Failed to query DB: %v", err)
	}
	if msg != "Test CLI Message" || title != "CLI Title" {
		t.Errorf("Expected 'Test CLI Message' and 'CLI Title', got '%s' and '%s'", msg, title)
	}
}

func TestRunCliClient(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/service", handleService(db))
	server := httptest.NewServer(mux)
	defer server.Close()

	addr := server.Listener.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	var stdout, stderr bytes.Buffer

	// Run client in background
	go runCliClient(ctx, addr, "text", "", "sess-456", "Test Session", "/tmp", "gemini", false, stdinReader, &stdout, &stderr)

	// Give it a moment to connect
	time.Sleep(200 * time.Millisecond)

	// Send a message through stdin
	fmt.Fprintln(stdinWriter, "Hello from CLI")
	
	// Broadcast a message from server to client
	time.Sleep(100 * time.Millisecond)
	interaction := Interaction{ID: 123, Title: "Server", Message: "Hello from Server", Timestamp: time.Now()}
	broadcaster.Broadcast(interaction)

	// Wait a bit for processing
	time.Sleep(200 * time.Millisecond)
	stdinWriter.Close()

	// Check if "Hello from CLI" reached the DB
	var count int
	db.QueryRow("SELECT COUNT(*) FROM interactions WHERE message = 'Hello from CLI'").Scan(&count)
	if count != 1 {
		t.Error("Message from CLI did not reach the database")
	}

	// Check if "Hello from Server" reached stdout
	if !strings.Contains(stdout.String(), "Hello from Server") {
		t.Errorf("Expected 'Hello from Server' in stdout, got: %s", stdout.String())
	}
}

func TestRunCliClientModes(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/service", handleService(db))
	server := httptest.NewServer(mux)
	defer server.Close()

	addr := server.Listener.Addr().String()

	// Test JSON mode
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	var stdout, stderr bytes.Buffer

	go runCliClient(ctx, addr, "json", "", "", "", "", "", false, stdinReader, &stdout, &stderr)
	time.Sleep(300 * time.Millisecond) // Wait for connection

	// Send JSON interaction via stdin
	interaction := Interaction{Message: "JSON Msg"}
	jsonInt, _ := json.Marshal(interaction)
	fmt.Fprintln(stdinWriter, string(jsonInt))
	time.Sleep(100 * time.Millisecond)
	
	// Broadcast one back
	broadcaster.Broadcast(Interaction{ID: 456, Message: "JSON Resp", Timestamp: time.Now()})
	
	// Wait a bit for processing
	time.Sleep(500 * time.Millisecond)
	stdinWriter.Close()

	if !strings.Contains(stdout.String(), `"message":"JSON Resp"`) {
		t.Errorf("Expected JSON response in stdout, got: %s", stdout.String())
	}

	// Test JSONR mode
	stdout.Reset()
	stderr.Reset()
	ctx2, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	stdinReader2, stdinWriter2 := io.Pipe()
	
	go runCliClient(ctx2, addr, "jsonr", "", "", "", "", "", false, stdinReader2, &stdout, &stderr)
	time.Sleep(300 * time.Millisecond)
	
	fmt.Fprintln(stdinWriter2, "Normal Msg")
	time.Sleep(100 * time.Millisecond)
	broadcaster.Broadcast(Interaction{ID: 789, Message: "JSONR Resp", Timestamp: time.Now()})
	
	time.Sleep(500 * time.Millisecond)
	stdinWriter2.Close()

	if !strings.Contains(stdout.String(), `"message":"JSONR Resp"`) {
		t.Errorf("Expected JSON response in stdout for jsonr mode, got: %s", stdout.String())
	}
}

func TestExportStatic(t *testing.T) {
	staticRoot, _ := fs.Sub(staticFS, "static")
	tempDir, err := os.MkdirTemp("", "push-export-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	err = exportStatic(staticRoot, tempDir, "Exported App", false, true)
	if err != nil {
		t.Fatalf("Failed to export static: %v", err)
	}

	// Verify files exist
	files := []string{"index.html", "manifest.json", "sw.js", "chat-app.jsx", "chat-messages.jsx"}
	for _, f := range files {
		if _, err := os.Stat(filepath.Join(tempDir, f)); err != nil {
			t.Errorf("File %s was not exported", f)
		}
	}

	// Verify content customization
	data, _ := os.ReadFile(filepath.Join(tempDir, "index.html"))
	if !strings.Contains(string(data), "<title>Exported App</title>") {
		t.Error("Exported index.html did not contain custom title")
	}
	if !strings.Contains(string(data), `{"interactive": true}`) {
		t.Error("Exported index.html did not contain interactive: true")
	}
}

func TestHandleStatic(t *testing.T) {
	staticRoot, _ := fs.Sub(staticFS, "static")
	handler := handleStatic(staticRoot, "Test Static App", false, false)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "<title>Test Static App</title>") {
		t.Error("index.html did not contain custom title")
	}

	// Test 404
	req = httptest.NewRequest("GET", "/notexists", nil)
	w = httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestLoadCustomIcons(t *testing.T) {
	tempDir, err := os.MkdirTemp("", "push-icon-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	iconPath := filepath.Join(tempDir, "icon.png")
	// Create a simple 512x512 PNG
	img := image.NewRGBA(image.Rect(0, 0, 512, 512))
	f, _ := os.Create(iconPath)
	png.Encode(f, img)
	f.Close()

	err = loadCustomIcons(iconPath)
	if err != nil {
		t.Fatalf("Failed to load custom icons: %v", err)
	}

	// Verify customIcons map
	expectedIcons := []string{"icon-128.png", "icon-192.png", "icon.png", "apple-touch-icon.png"}
	for _, name := range expectedIcons {
		if _, ok := customIcons[name]; !ok {
			t.Errorf("Icon %s was not loaded", name)
		}
	}

	// Test getStaticContent with custom icons
	staticRoot, _ := fs.Sub(staticFS, "static")
	data, contentType, _, err := getStaticContent(staticRoot, "/icon.png", "", true, false)
	if err != nil {
		t.Fatalf("Failed to get custom icon: %v", err)
	}
	if contentType != "image/png" {
		t.Errorf("Expected image/png, got %s", contentType)
	}
	if len(data) == 0 {
		t.Error("Icon data is empty")
	}
}

type streamRecorder struct {
	*httptest.ResponseRecorder
	data chan string
}

func (s *streamRecorder) Write(b []byte) (int, error) {
	lines := strings.Split(string(b), "\n")
	for _, line := range lines {
		if line != "" {
			s.data <- line
		}
	}
	return len(b), nil
}

func TestHandleServiceFiltering(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleService(db)

	// Client 1 with session filter
	req1 := httptest.NewRequest("GET", "/service?session_id=sess-1", nil)
	w1 := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), data: make(chan string, 10)}
	ctx1, cancel1 := context.WithCancel(context.Background())
	req1 = req1.WithContext(ctx1)
	go handler(w1, req1)

	// Client 2 with no session filter (main feed)
	req2 := httptest.NewRequest("GET", "/service", nil)
	w2 := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), data: make(chan string, 10)}
	ctx2, cancel2 := context.WithCancel(context.Background())
	req2 = req2.WithContext(ctx2)
	go handler(w2, req2)

	time.Sleep(100 * time.Millisecond)

	// 1. Global message (no session_id) - should reach both
	globalInt := Interaction{ID: 100, Message: "Global Msg"}
	broadcaster.Broadcast(globalInt)

	// 2. Session 1 message - should reach both (main feed shows all)
	sess1Int := Interaction{ID: 101, Message: "Sess 1 Msg", SessionID: "sess-1"}
	broadcaster.Broadcast(sess1Int)

	// 3. Session 2 message - should only reach main feed (w2)
	sess2Int := Interaction{ID: 102, Message: "Sess 2 Msg", SessionID: "sess-2"}
	broadcaster.Broadcast(sess2Int)

	time.Sleep(200 * time.Millisecond)

	// Check w1 (sess-1 filter)
	messages1 := drainStream(w1.data)
	if !containsMessage(messages1, "Global Msg") || !containsMessage(messages1, "Sess 1 Msg") {
		t.Errorf("Client 1 missing messages. Got: %v", messages1)
	}
	if containsMessage(messages1, "Sess 2 Msg") {
		t.Error("Client 1 should not have received Sess 2 Msg")
	}

	// Check w2 (no filter)
	messages2 := drainStream(w2.data)
	if !containsMessage(messages2, "Global Msg") || !containsMessage(messages2, "Sess 1 Msg") || !containsMessage(messages2, "Sess 2 Msg") {
		t.Errorf("Client 2 (Main Feed) missing messages. Got: %v", messages2)
	}

	cancel1()
	cancel2()
}

func drainStream(ch chan string) []string {
	var msgs []string
	for {
		select {
		case m := <-ch:
			msgs = append(msgs, m)
		default:
			return msgs
		}
	}
}

func containsMessage(msgs []string, text string) bool {
	for _, m := range msgs {
		if strings.Contains(m, text) {
			return true
		}
	}
	return false
}

func TestSessionActivityTracking(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleService(db)

	// Listen for broadcasts
	ch := broadcaster.Subscribe()
	defer broadcaster.Unsubscribe(ch)

	// Connect a client with session ID
	req := httptest.NewRequest("GET", "/service?session_id=test-session", nil)
	w := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), data: make(chan string, 10)}
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	done := make(chan struct{})
	go func() {
		handler(w, req)
		close(done)
	}()

	// Should receive session-active broadcast
	select {
	case i := <-ch:
		if i.Title != "session-active" || i.SessionID != "test-session" {
			t.Errorf("Expected session-active broadcast, got: %+v", i)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for session-active broadcast")
	}

	// Disconnect client
	cancel()
	<-done

	// Should receive session-inactive broadcast
	select {
	case i := <-ch:
		if i.Title != "session-inactive" || i.SessionID != "test-session" {
			t.Errorf("Expected session-inactive broadcast, got: %+v", i)
		}
	case <-time.After(1 * time.Second):
		t.Error("Timed out waiting for session-inactive broadcast")
	}
}

func TestHeartbeatActiveSessions(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Clear active sessions
	sessionsMu.Lock()
	activeSessions = make(map[string]int)
	sessionsMu.Unlock()

	handler := handleService(db)

	// Connect client 1
	req1 := httptest.NewRequest("GET", "/service?session_id=s1", nil)
	w1 := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), data: make(chan string, 20)}
	ctx1, cancel1 := context.WithCancel(context.Background())
	req1 = req1.WithContext(ctx1)
	go handler(w1, req1)

	time.Sleep(100 * time.Millisecond)

	// Connect client 2
	req2 := httptest.NewRequest("GET", "/service?session_id=s2", nil)
	w2 := &streamRecorder{ResponseRecorder: httptest.NewRecorder(), data: make(chan string, 20)}
	ctx2, cancel2 := context.WithCancel(context.Background())
	req2 = req2.WithContext(ctx2)
	go handler(w2, req2)

	time.Sleep(100 * time.Millisecond)

	// Verify heartbeats will contain both (if we waited for ticker, but let's check the map)
	sessionsMu.Lock()
	count := len(activeSessions)
	s1 := activeSessions["s1"]
	s2 := activeSessions["s2"]
	sessionsMu.Unlock()

	if count != 2 || s1 != 1 || s2 != 1 {
		t.Errorf("Expected 2 active sessions, got %d. s1=%d, s2=%d", count, s1, s2)
	}

	cancel1()
	cancel2()
}

func TestRunCliClientMetadata(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/service", handleService(db))
	server := httptest.NewServer(mux)
	defer server.Close()

	addr := server.Listener.Addr().String()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	stdinReader, stdinWriter := io.Pipe()
	var stdout, stderr bytes.Buffer

	// Run client with specific session, name, and model
	go runCliClient(ctx, addr, "text", "", "sess-999", "Special Session", "/tmp", "gemini-pro", false, stdinReader, &stdout, &stderr)

	// Give it a moment to connect and send registration
	time.Sleep(300 * time.Millisecond)

	// Verify registration message
	var reg Interaction
	err := db.QueryRow("SELECT title, message, agent, status, session_id FROM interactions WHERE title = 'session-register'").Scan(&reg.Title, &reg.Message, &reg.Agent, &reg.Status, &reg.SessionID)
	if err != nil {
		t.Errorf("Failed to find registration message: %v", err)
	}
	if reg.Agent != "gemini" {
		t.Errorf("Expected agent gemini, got %s", reg.Agent)
	}

	if reg.SessionID != "sess-999" {
		t.Errorf("Expected session_id sess-999, got %s", reg.SessionID)
	}
	if !strings.Contains(reg.Message, "Special Session") {
		t.Errorf("Expected message to contain 'Special Session', got %s", reg.Message)
	}
	if reg.Status != "r" {
		t.Errorf("Expected registration status 'r', got %q", reg.Status)
	}

	// Verify all registration messages have status "r"
	rows, err := db.Query("SELECT status FROM interactions WHERE title = 'session-register'")
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var status string
			if err := rows.Scan(&status); err == nil && status != "r" {
				t.Errorf("Expected all registration messages to have status 'r', got %q", status)
			}
		}
	}

	// Test Antigravity detection
	ctx2, cancel2 := context.WithCancel(context.Background())
	defer cancel2()
	var stdout2, stderr2 bytes.Buffer
	stdinReader2 := strings.NewReader("")
	go runCliClient(ctx2, addr, "text", "", "sess-456", "Agy Session", "/tmp", "agy", false, stdinReader2, &stdout2, &stderr2)
	time.Sleep(200 * time.Millisecond)
	cancel2()

	var agyMsg Interaction
	err = db.QueryRow("SELECT agent FROM interactions WHERE session_id = 'sess-456' AND title = 'session-register'").Scan(&agyMsg.Agent)
	if err != nil {
		t.Fatalf("Failed to find agy registration: %v", err)
	}
	if agyMsg.Agent != "antigravity" {
		t.Errorf("Expected agent antigravity, got %s", agyMsg.Agent)
	}

	// Send a normal text message
	fmt.Fprintln(stdinWriter, "Hello Metadata")
	time.Sleep(200 * time.Millisecond)
	stdinWriter.Close()

	// Verify text message has metadata
	var msg Interaction
	err = db.QueryRow("SELECT title, message, agent, session_id FROM interactions WHERE message = 'Hello Metadata'").Scan(&msg.Title, &msg.Message, &msg.Agent, &msg.SessionID)
	if err != nil {
		t.Errorf("Failed to find text message: %v", err)
	}
	if msg.Agent != "gemini" {
		t.Errorf("Text message missing agent gemini, got %s", msg.Agent)
	}
	if msg.Title != "Special Session" {
		t.Errorf("Text message missing title 'Special Session', got %s", msg.Title)
	}
	if msg.SessionID != "sess-999" {
		t.Errorf("Text message missing session_id sess-999, got %s", msg.SessionID)
	}
}

func TestUserMessagePushSuppression(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Intercept push notifications? Hard to do without refactoring.
	// But we can check if it at least saves correctly.
	handler := handleInteractions(db)

	interaction := Interaction{
		Message: "User Message",
		IsUser:  true,
	}
	body, _ := json.Marshal(interaction)
	req := httptest.NewRequest("POST", "/interactions", bytes.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("Expected status 201, got %d", w.Code)
	}

	var saved Interaction
	json.Unmarshal(w.Body.Bytes(), &saved)
	if !saved.IsUser {
		t.Error("Expected IsUser to be true in response")
	}

	// Verify in DB
	var isUser bool
	err := db.QueryRow("SELECT is_user FROM interactions WHERE id = ?", saved.ID).Scan(&isUser)
	if err != nil {
		t.Fatalf("Failed to query DB: %v", err)
	}
	if !isUser {
		t.Error("Expected is_user to be true in DB")
	}
}

func TestHandleInteractionsLatestPerSession(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	handler := handleInteractions(db)

	// 1. Insert messages for different sessions
	// Session A
	db.Exec("INSERT INTO interactions (message, session_id) VALUES (?, ?)", "Msg A1", "sess-a")
	db.Exec("INSERT INTO interactions (message, session_id) VALUES (?, ?)", "Msg A2", "sess-a")
	// Session B
	db.Exec("INSERT INTO interactions (message, session_id) VALUES (?, ?)", "Msg B1", "sess-b")
	// Main Feed (Empty session_id)
	db.Exec("INSERT INTO interactions (message, session_id) VALUES (?, ?)", "Main 1", "")
	db.Exec("INSERT INTO interactions (message, session_id) VALUES (?, ?)", "Main 2", "")

	// 2. Fetch latest per session
	req := httptest.NewRequest("GET", "/interactions?latest_per_session=true", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var results []Interaction
	if err := json.NewDecoder(w.Body).Decode(&results); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should have 3 results: A2, B1, and Main 2
	if len(results) != 3 {
		t.Errorf("Expected 3 results, got %d", len(results))
	}

	messages := make(map[string]string)
	for _, r := range results {
		messages[r.SessionID] = r.Message
	}

	if messages["sess-a"] != "Msg A2" {
		t.Errorf("Expected Msg A2 for sess-a, got %s", messages["sess-a"])
	}
	if messages["sess-b"] != "Msg B1" {
		t.Errorf("Expected Msg B1 for sess-b, got %s", messages["sess-b"])
	}
	if messages[""] != "Main 2" {
		t.Errorf("Expected Main 2 for empty session, got %s", messages[""])
	}
}

func TestRunHermesAgent(t *testing.T) {
	// 1. Mock Hermes Standard OpenAI Server
	hermesReceivedMsg := make(chan string, 1)
	hermesSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			t.Errorf("Expected POST request, got %s", r.Method)
			return
		}

		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("Expected path /v1/chat/completions, got %s", r.URL.Path)
			return
		}

		var body map[string]interface{}
		if err := json.NewDecoder(r.Body).Decode(&body); err == nil {
			msgs := body["messages"].([]interface{})
			lastMsg := msgs[len(msgs)-1].(map[string]interface{})
			hermesReceivedMsg <- lastMsg["content"].(string)
		}

		// Standard OpenAI SSE response
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Error("Streaming not supported")
			return
		}

		fmt.Fprintf(w, "data: {\"choices\": [{\"delta\": {\"content\": \"Hello \"}}]}\n\n")
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
		fmt.Fprintf(w, "event: hermes.tool.progress\ndata: {\"tool\": \"shell\", \"input\": \"ls\", \"status\": \"running\"}\n\n")
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
		fmt.Fprintf(w, "data: {\"choices\": [{\"delta\": {\"content\": \"World\"}}]}\n\n")
		flusher.Flush()
		time.Sleep(50 * time.Millisecond)
		fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer hermesSrv.Close()

	// 2. Mock Push Server
	pushReceivedMsgs := make(chan Interaction, 10)
	pushSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/service" && r.Method == "POST" {
			var i Interaction
			if err := json.NewDecoder(r.Body).Decode(&i); err == nil {
				pushReceivedMsgs <- i
			}
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path == "/service" && r.Method == "GET" {
			// Serve a user message after a short delay
			w.Header().Set("Content-Type", "application/x-ndjson")
			flusher, ok := w.(http.Flusher)
			if !ok {
				t.Error("Streaming not supported")
				return
			}
			time.Sleep(200 * time.Millisecond)
			i := Interaction{ID: 1, Message: "User Command", IsUser: true, SessionID: "hermes-test", Timestamp: time.Now()}
			data, _ := json.Marshal(i)
			w.Write(append(data, '\n'))
			flusher.Flush()
			// Keep connection open
			<-r.Context().Done()
			return
		}
	}))
	defer pushSrv.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// 3. Run Hermes Agent
	pushAddr := strings.TrimPrefix(pushSrv.URL, "http://")
	go runHermesAgent(ctx, hermesSrv.URL, pushAddr, "hermes-test", "Hermes Test Agent")

	// 4. Verify interactions
	var msgs []Interaction
loop:
	for {
		select {
		case m := <-pushReceivedMsgs:
			msgs = append(msgs, m)
			if len(msgs) >= 5 { // register, hello, shell progress, world, done(r)
				break loop
			}
		case <-ctx.Done():
			break loop
		}
	}

	// Check user message forwarding
	select {
	case hMsg := <-hermesReceivedMsg:
		if hMsg != "User Command" {
			t.Errorf("Expected Hermes to receive 'User Command', got '%s'", hMsg)
		}
	case <-time.After(2 * time.Second):
		t.Error("Timed out waiting for Hermes to receive message")
	}

	// Check parsing
	foundHello := false
	foundWorld := false
	foundTool := false
	foundDone := false

	for _, m := range msgs {
		if strings.Contains(m.Message, "Hello") {
			foundHello = true
		}
		if strings.Contains(m.Message, "World") {
			foundWorld = true
		}
		if strings.Contains(m.Message, "🔧 **shell**") {
			foundTool = true
		}
		if m.Status == "r" && strings.Contains(m.Message, "Hello World") {
			foundDone = true
		}
	}

	if !foundHello || !foundWorld || !foundTool || !foundDone {
		t.Errorf("Missing expected messages in proxy. Hello:%v, World:%v, Tool:%v, Done:%v", foundHello, foundWorld, foundTool, foundDone)
		for i, m := range msgs {
			t.Logf("Msg %d: %s (Status: %s, ID: %s)", i, m.Message, m.Status, m.Identifier)
		}
	}
}

func TestAgyScraper(t *testing.T) {
	// Create a temp file for the transcript
	tmpFile, err := os.CreateTemp("", "transcript_*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write initial lines
	lines := []string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-05-24T15:34:23Z","content":"<USER_REQUEST>\nHello Scraper\n</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-05-24T15:34:25Z","content":"I am planning to run a command.","thinking":"Let me check files first","tool_calls":[{"name":"run_command","args":{"CommandLine":"ls"}}]}`,
	}
	for _, l := range lines {
		if _, err := tmpFile.WriteString(l + "\n"); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
	}
	tmpFile.Sync()

	received := make(chan Interaction, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/interactions" && r.Method == "POST" {
			var i Interaction
			if err := json.NewDecoder(r.Body).Decode(&i); err == nil {
				received <- i
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	// Run scraper in background with yolo=false
	go runAgyScraper("", tmpFile.Name(), srv.URL, "test-session", "/test/path", "", false)

	// Wait and verify first batch of messages
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var firstBatch []Interaction
	expectedCount := 2 // User message (0), Model response (1)
	for len(firstBatch) < expectedCount {
		select {
		case i := <-received:
			firstBatch = append(firstBatch, i)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for first batch, got %d messages", len(firstBatch))
		}
	}

	// Verify details of first batch
	userMsgFound := false
	modelMsgFound := false
	for _, m := range firstBatch {
		if m.Identifier == "0" && m.IsUser && m.Kind == "status" && m.Message == "Hello Scraper" {
			userMsgFound = true
			if !m.Quiet {
				t.Error("Expected initial catch-up user message to be quiet")
			}
		}
		if m.Identifier == "1" && !m.IsUser && m.Kind == "agent" && m.Status == "w" {
			modelMsgFound = true
			if !m.Quiet {
				t.Error("Expected initial catch-up model message to be quiet")
			}
		}
	}
	if !userMsgFound || !modelMsgFound {
		t.Errorf("Missing expected messages in first batch. User:%v, Model:%v", userMsgFound, modelMsgFound)
	}

	// Wait for the scraper to hit EOF and exit catch-up mode
	time.Sleep(200 * time.Millisecond)

	// Append more lines to simulate agent progress
	moreLines := []string{
		`{"step_index":2,"source":"MODEL","type":"RUN_COMMAND","status":"DONE","created_at":"2026-05-24T15:34:26Z","content":"main.go\nmain_test.go"}`,
		`{"step_index":3,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-05-24T15:34:27Z","content":"I want to run another command.","thinking":"Let us do another step","tool_calls":[{"name":"run_command","args":{"CommandLine":"cat main.go"}}]}`,
	}
	for _, l := range moreLines {
		if _, err := tmpFile.WriteString(l + "\n"); err != nil {
			t.Fatalf("Failed to write more to temp file: %v", err)
		}
	}
	tmpFile.Sync()

	// Wait and verify second batch of messages
	var secondBatch []Interaction
	expectedCount2 := 2 // Tool output (2), Final model response (3)
	for len(secondBatch) < expectedCount2 {
		select {
		case i := <-received:
			secondBatch = append(secondBatch, i)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for second batch, got %d messages", len(secondBatch))
		}
	}

	toolMsgFound := false
	finalMsgFound := false
	for _, m := range secondBatch {
		if m.Identifier == "2" && m.Kind == "tool" && m.Message == "main.go\nmain_test.go" && m.Status == "d" {
			toolMsgFound = true
			if !m.Quiet {
				// Regular message is quiet by default
				t.Error("Expected real-time regular tool message to still be quiet")
			}
		}
		if m.Identifier == "3" && m.Kind == "agent" && m.Status == "w" {
			finalMsgFound = true
			if !m.Quiet {
				// Regular message is quiet by default
				t.Error("Expected real-time regular model message to still be quiet")
			}
		}
	}
	if !toolMsgFound || !finalMsgFound {
		t.Errorf("Missing expected messages in second batch. Tool:%v, Final:%v", toolMsgFound, finalMsgFound)
	}
}

func TestUserMessageDeduplication(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// 1. Save original user message from Web UI (no identifier)
	orig := Interaction{
		IsUser:    true,
		Message:   "Hello World",
		SessionID: "sess-abc",
	}
	err := saveInteraction(db, &orig)
	if err != nil {
		t.Fatalf("Failed to save original: %v", err)
	}
	if orig.ID == 0 {
		t.Fatal("Expected non-zero ID for original")
	}
	if orig.Identifier != "" {
		t.Errorf("Expected empty identifier for original, got '%s'", orig.Identifier)
	}

	// 2. Save scraped user message (has identifier and same content)
	scraped := Interaction{
		IsUser:          true,
		Message:         "Hello World",
		DetailedMessage: "Hello World",
		Identifier:      "step-0",
		SessionID:       "sess-abc",
	}
	err = saveInteraction(db, &scraped)
	if err != nil {
		t.Fatalf("Failed to save scraped: %v", err)
	}
	if scraped.ID != orig.ID {
		t.Errorf("Expected scraped message to update original ID %d, but got ID %d", orig.ID, scraped.ID)
	}

	// Verify DB has only 1 row
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM interactions WHERE session_id = 'sess-abc'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected exactly 1 interaction, got %d", count)
	}

	// Verify identifier was updated
	var ident string
	err = db.QueryRow("SELECT identifier FROM interactions WHERE id = ?", orig.ID).Scan(&ident)
	if err != nil {
		t.Fatalf("Failed to query identifier: %v", err)
	}
	if ident != "step-0" {
		t.Errorf("Expected identifier to be 'step-0', got '%s'", ident)
	}
}

func TestGeminiAgentScriptCleanup(t *testing.T) {
	if !strings.Contains(geminiAgentScript, "cleanup()") {
		t.Error("gemini-agent script does not contain cleanup() function definition")
	}
	if !strings.Contains(geminiAgentScript, "trap cleanup EXIT") {
		t.Error("gemini-agent script does not set trap for EXIT")
	}
	if !strings.Contains(geminiAgentScript, "kill \"$PUSH_PID\"") && !strings.Contains(geminiAgentScript, "kill $PUSH_PID") {
		t.Error("gemini-agent script does not kill PUSH_PID during cleanup")
	}
	if !strings.Contains(geminiAgentScript, "PUSH_PID_FILE=\"/tmp/push-client-$$.pid\"") {
		t.Error("gemini-agent script does not initialize PUSH_PID_FILE using parent shell PID ($$)")
	}
	if !strings.Contains(geminiAgentScript, "PID=$(cat \"$PUSH_PID_FILE\" 2>/dev/null)") {
		t.Error("gemini-agent script does not read PID from PUSH_PID_FILE during cleanup")
	}
	if !strings.Contains(geminiAgentScript, "kill \"$PID\"") {
		t.Error("gemini-agent script does not kill the PID from PUSH_PID_FILE during cleanup")
	}
	if !strings.Contains(geminiAgentScript, "$count -lt 10") {
		t.Error("gemini-agent script does not limit process wait loops to prevent hangs")
	}
	if !strings.Contains(geminiAgentScript, "kill -9") {
		t.Error("gemini-agent script does not fall back to SIGKILL for stuck processes")
	}
}

func TestAgySessionIsolation(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// 1. Save an interaction for session A
	iA := Interaction{
		Identifier: "step-10",
		SessionID:  "sess-a",
		Message:    "Message from A",
		IsUser:     false,
	}
	err := saveInteraction(db, &iA)
	if err != nil {
		t.Fatalf("Failed to save session A: %v", err)
	}

	// 2. Save an interaction for session B with the SAME identifier
	iB := Interaction{
		Identifier: "step-10",
		SessionID:  "sess-b",
		Message:    "Message from B",
		IsUser:     false,
	}
	err = saveInteraction(db, &iB)
	if err != nil {
		t.Fatalf("Failed to save session B: %v", err)
	}

	// 3. Verify they are separate rows (different IDs)
	if iA.ID == iB.ID {
		t.Errorf("Expected different IDs for separate sessions, but both got ID %d", iA.ID)
	}

	// 4. Verify count of interactions in DB is 2
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM interactions").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}
	if count != 2 {
		t.Errorf("Expected exactly 2 interactions in database, got %d", count)
	}
}

func TestTranslateAgentArgs(t *testing.T) {
	tests := []struct {
		name          string
		isAntigravity bool
		resume        bool
		yolo          bool
		extraArgs     []string
		expected      []string
	}{
		{
			name:          "gemini resume",
			isAntigravity: false,
			resume:        true,
			yolo:          false,
			extraArgs:     []string{"my-session"},
			expected:      []string{"--resume", "my-session"},
		},
		{
			name:          "antigravity resume (via resume bool)",
			isAntigravity: true,
			resume:        true,
			yolo:          false,
			extraArgs:     []string{"my-session"},
			expected:      []string{"--agent", "agy", "--continue", "my-session"},
		},
		{
			name:          "antigravity resume via extraArgs",
			isAntigravity: true,
			resume:        false,
			yolo:          false,
			extraArgs:     []string{"my-session", "--resume"},
			expected:      []string{"--agent", "agy", "my-session", "--continue"},
		},
		{
			name:          "antigravity resume via em-dash",
			isAntigravity: true,
			resume:        false,
			yolo:          false,
			extraArgs:     []string{"my-session", "—resume"},
			expected:      []string{"--agent", "agy", "my-session", "--continue"},
		},
		{
			name:          "gemini continue via extraArgs",
			isAntigravity: false,
			resume:        false,
			yolo:          false,
			extraArgs:     []string{"my-session", "--continue"},
			expected:      []string{"my-session", "--resume"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual := translateAgentArgs(tt.isAntigravity, tt.resume, tt.yolo, tt.extraArgs)
			if len(actual) != len(tt.expected) {
				t.Fatalf("expected %v, got %v", tt.expected, actual)
			}
			for i, v := range actual {
				if v != tt.expected[i] {
					t.Errorf("at index %d: expected %s, got %s", i, tt.expected[i], v)
				}
			}
		})
	}
}

func TestGeminiAgentScriptContinueAlias(t *testing.T) {
	if !strings.Contains(geminiAgentScript, "--resume|--continue)") {
		t.Error("gemini-agent script does not contain --resume|--continue) alias parsing")
	}
}

func TestGeminiAgentScriptNoInfoLogging(t *testing.T) {
	silencedLogs := []string{
		`echo "Resuming session:`,
		`echo "Started session:`,
		`echo "Initializing new session..."`,
		`echo "Started internal agy log scraper`,
		`echo "Restarting and resuming session..."`,
		`echo "Restarting fresh..."`,
	}
	for _, log := range silencedLogs {
		if strings.Contains(geminiAgentScript, log) {
			t.Errorf("gemini-agent script should not log normal operational messages: %q", log)
		}
	}
}

func TestAgyScraperYolo(t *testing.T) {
	// Create a temp file for the transcript
	tmpFile, err := os.CreateTemp("", "transcript_yolo_*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write initial lines
	lines := []string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-05-24T15:34:23Z","content":"<USER_REQUEST>\nHello Scraper\n</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-05-24T15:34:25Z","content":"I am planning to run a command.","thinking":"Let me check files first","tool_calls":[{"name":"run_command","args":{"CommandLine":"ls"}}]}`,
	}
	for _, l := range lines {
		if _, err := tmpFile.WriteString(l + "\n"); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
	}
	tmpFile.Sync()

	received := make(chan Interaction, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/interactions" && r.Method == "POST" {
			var i Interaction
			if err := json.NewDecoder(r.Body).Decode(&i); err == nil {
				received <- i
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	// Run scraper in background with yolo=true to test push notification and approval card suppression
	go runAgyScraper("", tmpFile.Name(), srv.URL, "test-session", "/test/path", "", true)

	// Wait and verify first batch of messages
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()

	var firstBatch []Interaction
	expectedCount := 2 // User message (0), Model response (1), NO approval card
	for len(firstBatch) < expectedCount {
		select {
		case i := <-received:
			firstBatch = append(firstBatch, i)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for first batch, got %d messages (expected 2)", len(firstBatch))
		}
	}

	// Verify details of first batch
	userMsgFound := false
	modelMsgFound := false
	approvalCardFound := false
	for _, m := range firstBatch {
		if m.Identifier == "0" && m.IsUser && m.Kind == "status" && m.Message == "Hello Scraper" {
			userMsgFound = true
		}
		if m.Identifier == "1" && !m.IsUser && m.Kind == "agent" && m.Status == "w" {
			modelMsgFound = true
		}
		if m.Identifier == "1-approval" || strings.Contains(m.Title, "ToolPermission") || m.Kind == "approval" {
			approvalCardFound = true
		}
	}
	if !userMsgFound || !modelMsgFound {
		t.Errorf("Missing expected messages in first batch. User:%v, Model:%v", userMsgFound, modelMsgFound)
	}
	if approvalCardFound {
		t.Error("Did not expect any approval card / ToolPermission message in YOLO mode")
	}

	// Wait a bit more to ensure no late approval cards are sent
	select {
	case m := <-received:
		if m.Identifier == "1-approval" || strings.Contains(m.Title, "ToolPermission") || m.Kind == "approval" {
			t.Error("Received late approval card in YOLO mode")
		}
	case <-time.After(100 * time.Millisecond):
		// Clean exit, no additional messages received
	}
}

func TestRenameSession(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// Setup active session
	sessionsMu.Lock()
	activeSessions["old-session-123"] = 1
	sessionsMu.Unlock()

	defer func() {
		sessionsMu.Lock()
		delete(activeSessions, "old-session-123")
		delete(activeSessions, "new-session-456")
		sessionsMu.Unlock()
	}()

	// Insert test interaction
	interaction := Interaction{
		Message:   "Hello Rename",
		SessionID: "old-session-123",
	}
	err := saveInteraction(db, &interaction)
	if err != nil {
		t.Fatalf("Failed to save interaction: %v", err)
	}

	// Subscribe to the broadcaster
	ch := broadcaster.Subscribe()
	defer broadcaster.Unsubscribe(ch)

	handler := handleRenameSession(db)

	// Call handler with missing params
	reqErr := httptest.NewRequest("POST", "/rename-session", nil)
	wErr := httptest.NewRecorder()
	handler(wErr, reqErr)
	if wErr.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing params, got %d", wErr.Code)
	}

	// Call handler to rename session
	req := httptest.NewRequest("POST", "/rename-session?old=old-session-123&new=new-session-456", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Verify broadcaster received rename message
	select {
	case msg := <-ch:
		if msg.Title != "session-rename" {
			t.Errorf("Expected broadcast title 'session-rename', got '%s'", msg.Title)
		}
		if msg.Message != "old-session-123" {
			t.Errorf("Expected broadcast message 'old-session-123', got '%s'", msg.Message)
		}
		if msg.SessionID != "new-session-456" {
			t.Errorf("Expected broadcast session_id 'new-session-456', got '%s'", msg.SessionID)
		}
	case <-time.After(500 * time.Millisecond):
		t.Errorf("Timed out waiting for rename broadcast message")
	}

	// Verify activeSessions was updated
	sessionsMu.Lock()
	oldVal := activeSessions["old-session-123"]
	newVal := activeSessions["new-session-456"]
	sessionsMu.Unlock()
	if oldVal != 0 {
		t.Errorf("Expected old-session-123 to be removed from activeSessions, got count %d", oldVal)
	}
	if newVal != 1 {
		t.Errorf("Expected new-session-456 to have count 1 in activeSessions, got %d", newVal)
	}

	// Verify database was updated
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM interactions WHERE session_id = ?", "new-session-456").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected 1 interaction with new session_id, got %d", count)
	}

	err = db.QueryRow("SELECT COUNT(*) FROM interactions WHERE session_id = ?", "old-session-123").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query database: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected 0 interactions with old session_id, got %d", count)
	}
}

func TestNormalizeArgs(t *testing.T) {
	tests := []struct {
		name     string
		input    []string
		expected []string
	}{
		{
			name:     "no dashes",
			input:    []string{"cmd", "arg1", "arg2"},
			expected: []string{"cmd", "arg1", "arg2"},
		},
		{
			name:     "em-dash flags",
			input:    []string{"cmd", "—antigravity", "—yolo"},
			expected: []string{"cmd", "--antigravity", "--yolo"},
		},
		{
			name:     "en-dash flags",
			input:    []string{"cmd", "–antigravity", "–yolo"},
			expected: []string{"cmd", "--antigravity", "--yolo"},
		},
		{
			name:     "mixed dash types and standard flags",
			input:    []string{"cmd", "-standard", "--double", "—em", "–en"},
			expected: []string{"cmd", "-standard", "--double", "--em", "--en"},
		},
		{
			name:     "skip executable path normalization even if it starts with dash",
			input:    []string{"—cmd", "—arg"},
			expected: []string{"—cmd", "--arg"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := normalizeArgs(tt.input)
			if len(result) != len(tt.expected) {
				t.Fatalf("Length mismatch: got %d, expected %d", len(result), len(tt.expected))
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Errorf("Mismatch at index %d: got %s, expected %s", i, result[i], tt.expected[i])
				}
			}
		})
	}
}

func TestAgyScraperQuestion(t *testing.T) {
	// Create a temp file for the transcript
	tmpFile, err := os.CreateTemp("", "transcript_question_*.jsonl")
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	// Write mock transcript lines with ask_question tool call
	lines := []string{
		`{"step_index":0,"source":"USER_EXPLICIT","type":"USER_INPUT","status":"DONE","created_at":"2026-05-24T15:34:23Z","content":"<USER_REQUEST>\nTest Question\n</USER_REQUEST>"}`,
		`{"step_index":1,"source":"MODEL","type":"PLANNER_RESPONSE","status":"DONE","created_at":"2026-05-24T15:34:25Z","content":"Let me ask a question.","thinking":"Need more info","tool_calls":[{"name":"ask_question","args":{"questions":[{"question":"Which path?","options":["A","B"],"is_multi_select":false}]}}]}`,
	}
	for _, l := range lines {
		if _, err := tmpFile.WriteString(l + "\n"); err != nil {
			t.Fatalf("Failed to write to temp file: %v", err)
		}
	}
	tmpFile.Sync()

	received := make(chan Interaction, 10)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/interactions" && r.Method == "POST" {
			var i Interaction
			if err := json.NewDecoder(r.Body).Decode(&i); err == nil {
				received <- i
			}
			w.WriteHeader(http.StatusOK)
		}
	}))
	defer srv.Close()

	// Run scraper in background
	go runAgyScraper("", tmpFile.Name(), srv.URL, "test-session", "/test/path", "", false)

	// Wait and verify messages
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	var interactions []Interaction
	expectedCount := 3 // User status (0), Model response (1), Question card (1-question)
	for len(interactions) < expectedCount {
		select {
		case i := <-received:
			interactions = append(interactions, i)
		case <-ctx.Done():
			t.Fatalf("Timeout waiting for interactions, got %d (expected %d)", len(interactions), expectedCount)
		}
	}

	// Verify that the question card interaction was generated correctly
	var questionCard *Interaction
	for i := range interactions {
		if interactions[i].Kind == "question" {
			questionCard = &interactions[i]
		}
	}

	if questionCard == nil {
		t.Fatal("Failed to find generated question card")
	}

	if questionCard.Identifier != "1-question" {
		t.Errorf("Expected identifier '1-question', got %s", questionCard.Identifier)
	}

	if questionCard.Title != "Question" {
		t.Errorf("Expected title 'Question', got %s", questionCard.Title)
	}

	// Verify the detailed message JSON
	var payload struct {
		Questions []struct {
			Header   string `json:"header"`
			Question string `json:"question"`
			Type     string `json:"type"`
			Options  []struct {
				Label string `json:"label"`
			} `json:"options"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(questionCard.DetailedMessage), &payload); err != nil {
		t.Fatalf("Failed to parse question card detailed message JSON: %v", err)
	}

	if len(payload.Questions) != 1 {
		t.Fatalf("Expected 1 question, got %d", len(payload.Questions))
	}

	q := payload.Questions[0]
	if q.Question != "Which path?" {
		t.Errorf("Expected question 'Which path?', got %s", q.Question)
	}
	if q.Type != "choice" {
		t.Errorf("Expected type 'choice', got %s", q.Type)
	}
	if len(q.Options) != 2 || q.Options[0].Label != "A" || q.Options[1].Label != "B" {
		t.Errorf("Expected options A and B, got options: %v", q.Options)
	}
}

func TestRunCliClientTmuxAgent(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/service", handleService(db))
	server := httptest.NewServer(mux)
	defer server.Close()

	addr := server.Listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdinReader := strings.NewReader("")
	var stdout, stderr bytes.Buffer

	// Start in tmux mode, targeting a pane, specifying model 'agy'
	go runCliClient(ctx, addr, "tmux", "test:0.0", "sess-tmux-123", "Tmux Agy Session", "/tmp", "agy", false, stdinReader, &stdout, &stderr)

	// Wait for the client to register and send forwarding message
	time.Sleep(300 * time.Millisecond)

	// Kill the client (which should trigger exit message)
	cancel()
	time.Sleep(200 * time.Millisecond)

	// Verify messages in DB
	rows, err := db.Query("SELECT title, agent, message FROM interactions WHERE session_id = 'sess-tmux-123'")
	if err != nil {
		t.Fatalf("Failed to query interactions: %v", err)
	}
	defer rows.Close()

	var hasReg, hasForward, hasExit bool
	for rows.Next() {
		var title, agent, message string
		if err := rows.Scan(&title, &agent, &message); err != nil {
			t.Fatalf("Failed to scan row: %v", err)
		}
		if title == "session-register" {
			hasReg = true
			if agent != "antigravity" {
				t.Errorf("Expected registration agent to be 'antigravity', got %q", agent)
			}
		} else if strings.Contains(message, "Now forwarding responses") {
			hasForward = true
			if agent != "antigravity" {
				t.Errorf("Expected forwarding message agent to be 'antigravity', got %q", agent)
			}
		} else if strings.Contains(message, "No longer forwarding responses") {
			hasExit = true
			if agent != "antigravity" {
				t.Errorf("Expected exit message agent to be 'antigravity', got %q", agent)
			}
		}
	}

	if !hasReg {
		t.Error("Missing session-register message")
	}
	if !hasForward {
		t.Error("Missing 'Now forwarding responses' message")
	}
	if !hasExit {
		t.Error("Missing 'No longer forwarding responses' message")
	}
}

func TestParsePaneQuestion(t *testing.T) {
	paneContent := `Question
────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────────

Question 1/1: Which database system do you prefer for small-to-medium scale Go projects?

> 1. SQLite (embedded, single-file)
  2. PostgreSQL (robust, feature-rich relational)
  3. MySQL / MariaDB
  4. Redis / Key-Value store
  5. MongoDB / Document-based
  6. Write-in...

  ↑/↓ Navigate · enter Select · esc Skip
`
	question, options, ok, isToolPermission := parsePaneQuestion(paneContent)
	if !ok {
		t.Fatalf("Expected parsePaneQuestion to succeed, but it failed")
	}
	if isToolPermission {
		t.Errorf("Expected isToolPermission to be false, got true")
	}

	expectedQuestion := "Which database system do you prefer for small-to-medium scale Go projects?"
	if question != expectedQuestion {
		t.Errorf("Expected question %q, got %q", expectedQuestion, question)
	}

	expectedOptions := []string{
		"SQLite (embedded, single-file)",
		"PostgreSQL (robust, feature-rich relational)",
		"MySQL / MariaDB",
		"Redis / Key-Value store",
		"MongoDB / Document-based",
		"Write-in...",
	}
	if len(options) != len(expectedOptions) {
		t.Fatalf("Expected %d options, got %d", len(expectedOptions), len(options))
	}

	for i, opt := range options {
		if opt != expectedOptions[i] {
			t.Errorf("Option %d: expected %q, got %q", i, expectedOptions[i], opt)
		}
	}
}

func TestParsePaneToolPermission(t *testing.T) {
	paneContent := `Action: command
Target: git commit -m "Fix local syntax error in gemini-agent wrapper script"
Reason: Commit changes to Git

1. Grant permission
2. Grant permission for the rest of this session
3. Deny permission

  ↑/↓ Navigate · enter Select · esc Skip
`
	question, options, ok, isToolPermission := parsePaneQuestion(paneContent)
	if !ok {
		t.Fatalf("Expected parsePaneQuestion to succeed, but it failed")
	}
	if !isToolPermission {
		t.Errorf("Expected isToolPermission to be true, got false")
	}

	expectedQuestion := "Action: command\nTarget: git commit -m \"Fix local syntax error in gemini-agent wrapper script\"\nReason: Commit changes to Git"
	if question != expectedQuestion {
		t.Errorf("Expected question:\n%q\nGot:\n%q", expectedQuestion, question)
	}

	expectedOptions := []string{
		"Grant permission",
		"Grant permission for the rest of this session",
		"Deny permission",
	}
	if len(options) != len(expectedOptions) {
		t.Fatalf("Expected %d options, got %d", len(expectedOptions), len(options))
	}

	for i, opt := range options {
		if opt != expectedOptions[i] {
			t.Errorf("Option %d: expected %q, got %q", i, expectedOptions[i], opt)
		}
	}
}


func TestRunCliClientTmuxChoice(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	mux := http.NewServeMux()
	mux.HandleFunc("/service", handleService(db))
	server := httptest.NewServer(mux)
	defer server.Close()

	addr := server.Listener.Addr().String()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stdinReader := strings.NewReader("")
	var stdout, stderr bytes.Buffer

	// Start in tmux mode
	go runCliClient(ctx, addr, "tmux", "nonexistent-session:0.0", "sess-tmux-choice", "Tmux Choice Test", "/tmp", "agy", false, stdinReader, &stdout, &stderr)

	// Wait for registration
	time.Sleep(300 * time.Millisecond)

	// Clear stderr
	stderr.Reset()

	// 1. Post a normal (non-choice) user message. It should try to send keys AND Enter.
	iNormal := Interaction{
		Message:   "hello",
		IsUser:    true,
		SessionID: "sess-tmux-choice",
	}
	saveInteraction(db, &iNormal)
	broadcaster.Broadcast(iNormal)

	// Wait for the message to be processed by runCliClient
	time.Sleep(600 * time.Millisecond)

	normalErr := stderr.String()
	if !strings.Contains(normalErr, "Failed to send Enter to tmux") {
		t.Errorf("Expected normal message to trigger 'Failed to send Enter to tmux', got stderr: %q", normalErr)
	}

	// Clear stderr
	stderr.Reset()

	// 2. Post a choice user message. It should try to send keys but NOT Enter.
	iChoice := Interaction{
		Message:   "3",
		IsUser:    true,
		Kind:      "choice",
		SessionID: "sess-tmux-choice",
	}
	saveInteraction(db, &iChoice)
	broadcaster.Broadcast(iChoice)

	time.Sleep(300 * time.Millisecond)

	choiceErr := stderr.String()
	if !strings.Contains(choiceErr, "Failed to send keys to tmux") {
		t.Errorf("Expected choice message to try sending keys, got stderr: %q", choiceErr)
	}
	if strings.Contains(choiceErr, "Failed to send Enter to tmux") {
		t.Errorf("Expected choice message NOT to send Enter to tmux, but it did! Stderr: %q", choiceErr)
	}
}

func TestSaveInteractionConsecutiveReady(t *testing.T) {
	db, tempDir := setupTestDB(t)
	defer os.RemoveAll(tempDir)
	defer db.Close()

	// 1. Save a "Ready" message
	i1 := Interaction{
		SessionID: "sess-ready-test",
		Kind:      "status",
		Status:    "r",
		Message:   "Ready",
	}
	err := saveInteraction(db, &i1)
	if err != nil {
		t.Fatalf("Failed to save first Ready message: %v", err)
	}

	// 2. Save a consecutive "Ready" message
	i2 := Interaction{
		SessionID: "sess-ready-test",
		Kind:      "status",
		Status:    "r",
		Message:   "Ready",
	}
	err = saveInteraction(db, &i2)
	if err != nil {
		t.Fatalf("Failed to save second Ready message: %v", err)
	}

	// 3. Verify they were not duplicated (count in DB should be 1)
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM interactions WHERE session_id = 'sess-ready-test'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count: %v", err)
	}
	if count != 1 {
		t.Errorf("Expected exactly 1 interaction in database, got %d", count)
	}

	// 4. Save a different message in between
	i3 := Interaction{
		SessionID: "sess-ready-test",
		Kind:      "agent",
		Status:    "w",
		Message:   "Thinking...",
	}
	err = saveInteraction(db, &i3)
	if err != nil {
		t.Fatalf("Failed to save intermediate agent message: %v", err)
	}

	// 5. Save another "Ready" message (now non-consecutive)
	i4 := Interaction{
		SessionID: "sess-ready-test",
		Kind:      "status",
		Status:    "r",
		Message:   "Ready",
	}
	err = saveInteraction(db, &i4)
	if err != nil {
		t.Fatalf("Failed to save third Ready message: %v", err)
	}

	// 6. Verify it was successfully inserted (total count should now be 3)
	err = db.QueryRow("SELECT COUNT(*) FROM interactions WHERE session_id = 'sess-ready-test'").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query count after third message: %v", err)
	}
	if count != 3 {
		t.Errorf("Expected exactly 3 interactions in database, got %d", count)
	}
}

func TestStaticAssetsContainsToolDenyLogic(t *testing.T) {
	data, err := staticFS.ReadFile("static/chat-app.jsx")
	if err != nil {
		t.Fatalf("Failed to read static/chat-app.jsx: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "isToolDenyChoice") {
		t.Error("static/chat-app.jsx does not contain 'isToolDenyChoice' logic")
	}
	if !strings.Contains(content, "isToolPermission && isLast") {
		t.Error("static/chat-app.jsx does not contain immediate ready state transition on tool deny")
	}
}






