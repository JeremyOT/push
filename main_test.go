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
		// It might be the first message (Service Message) or the "Stream me" one.
		// Since Service Message was saved to DB, it might be sent as initial data.
		if received.Message != "Service Message" && received.Message != "Stream me" {
			t.Errorf("Unexpected message: %s", received.Message)
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
	go runCliClient(ctx, addr, "text", "", "sess-456", "Test Session", "gemini", stdinReader, &stdout, &stderr)

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

	go runCliClient(ctx, addr, "json", "", "", "", "", stdinReader, &stdout, &stderr)
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
	
	go runCliClient(ctx2, addr, "jsonr", "", "", "", "", stdinReader2, &stdout, &stderr)
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
