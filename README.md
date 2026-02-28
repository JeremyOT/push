Push
====

Push is a lightweight one way notification system written in Go that serves a webpage over http, displaying a local database of interactions/messages as a one way chat in an imessage style interface.

It uses the Push API (https://developer.mozilla.org/en-US/docs/Web/API/Push_API) to notify consumers of new interactions when offline, and dynamically updates the chat view as new interactions are posted.

Usage
-----

### Running the Server

Start the server with default settings:
```bash
./push
```

Custom configuration:
```bash
./push --address=127.0.0.1:8089 --database=./push.sqlite --interactive
```

### Full Flags List

| Flag | Description | Default |
|------|-------------|---------|
| `--address` | Address and port to listen on | `127.0.0.1:8089` |
| `--database` | Path to the SQLite database file | `./push.sqlite` |
| `--hostname` | Hostname used for VAPID/Web Push notifications | `os.Hostname()` |
| `--interactive` | Enable the "Send" button in the web interface | `false` |
| `--application-title` | Custom title for the web application | `Push` |
| `--icon` | Path to a PNG file to use for all app icons | (embedded defaults) |
| `--static-output` | Directory to export the customized static web app | (none) |
| `--reset-vapid` | Delete existing VAPID keys from the database | `false` |
| `--m` | Message content (triggers client mode to send a message) | `""` |
| `--t` | Title for the message (used with `--m`) | `""` |
| `--cli-service` | Enable interactive CLI mode | `""` |
| `--tmux-target` | Target pane for `tmux` mode (e.g., `session:window.pane`) | `""` |

### Sending Messages from CLI

You can send a quick message without starting the server or a persistent CLI session:
```bash
./push --address=localhost:8089 -t "System Alert" -m "Memory usage is high"
```

Web Customization
----------------

Push allows you to customize the web interface:

- **Custom Title**: Use `--application-title="My Home Dashboard"` to change the header.
- **Custom Icon**: Use `--icon=path/to/icon.png`. The server will automatically resize and serve it at all required dimensions (16x16 up to 512x512).
- **Static Export**: Use `--static-output=./dist` to save the fully rendered web app (including your custom title and icons) to a directory. This is useful for serving via Nginx or other static hosts.

Services & Advanced Usage
-------------------------

### 1. Simple Messaging (The `/interactions` Endpoint)

The easiest way to send a message is using a standard JSON POST request:

```bash
curl -X POST \
     -H "Content-Type: application/json" \
     -d '{"message": "Hello World", "title": "My Service"}' \
     "http://localhost:8089/interactions"
```

### 2. Streaming Service (The `/service` Endpoint)

For real-time integration, the `/service` endpoint uses Newline Delimited JSON (NDJSON) to stream messages.

*   **Receive Stream:** `GET /service` will keep the connection open and stream new interactions as they occur.
    *   Use `GET /service?timestamp=2026-02-26T12:00:00Z` to receive messages since a specific time (RFC3339 or `YYYY-MM-DD HH:MM:SS`).
*   **Send Messages:** `POST /service?stream=false` allows sending a message via NDJSON without opening a stream.
*   **Bi-directional Stream:** `POST /service` allows you to both send and receive messages over a single persistent connection.

### 3. CLI Service Mode

The `push` binary includes a built-in CLI client to interact with a running Push server. This client features **automatic reconnection with exponential backoff** and tracks message timestamps to ensure no messages are missed during temporary connection loss.

```bash
./push --address=localhost:8089 --cli-service=[MODE]
```

**Modes:**
*   `text` (Default): An interactive chat-like interface.
*   `json`: Outputs each received message as a JSON object on a new line. It also expects JSON input for sending messages. Ideal for piping to tools like `jq`.
*   `jsonr`: Interactive mode like `text` but with NDJSON output.
*   `tmux`: Forwards user messages received from the web app to a specified tmux pane. Requires `--tmux-target`.
*   `tmux:client_id`: Like `tmux` mode, but only processes messages prefixed with `client_id ` or `client_id: `. The prefix is stripped before forwarding.

### 4. Tmux Integration

The `tmux` mode allows you to forward messages directly into a tmux pane. This is useful for remote command execution or providing input to a running process from the Push web interface.

```bash
./push --address=localhost:8089 --cli-service=tmux --tmux-target="mysession:window.0"
```

Implementation
--------------

The implementation is a single binary written in Go with embedded html/javascript/css using Golang's embed package. Interactions are stored in a local sqlite database.
