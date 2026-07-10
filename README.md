Push
====

Push is a lightweight one way notification system written in Go that serves a webpage over http, displaying a local database of interactions/messages as a one way chat in an imessage style interface.

It uses the Push API (https://developer.mozilla.org/en-US/docs/Web/API/Push_API) to notify consumers of new interactions when offline, and dynamically updates the chat view as new interactions are posted.

Usage
-----

### Deployment

A `deploy.sh` script is provided to build and install the `push` binary to your local bin directory and restart the service:

```bash
./deploy.sh
```

This script:
1.  Builds the project with optimized flags (`-w -s`).
2.  Moves the resulting `push` binary to `~/bin/push`.
3.  Executes `serve-push` to restart the service (ensure `serve-push` is in your `PATH`).

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
| `--cli-service` | Enable interactive CLI mode (`text`, `json`, `jsonr`, `tmux`) | `""` |
| `--tmux-target` | Target pane for `tmux` mode (e.g., `%1` or `session:window.pane`) | `""` |
| `--session-id` | Unique ID for the current CLI session | `""` |
| `--session-name` | Display name for the session in the web UI | `""` |
| `--model` | Model name associated with the session (e.g., `gemini`) | `""` |
| `--signal-server` | Signal CLI REST API daemon host:port (e.g., `127.0.0.1:8742`) | `""` |
| `--signal-address` | Phone number registered with signal-cli (e.g., `+1234567890`) | `""` |
| `--signal` | Immediately enable Signal forwarding (can be `true` or `quiet`) | `""` |

### Sending Messages from CLI

You can send a quick message without starting the server or a persistent CLI session:
```bash
./push --address=localhost:8089 -t "System Alert" -m "Memory usage is high"
```

Gemini & Antigravity Agent Integration
------------------------------------

The `push` binary includes the `gemini-agent` logic embedded directly, providing a seamless way to connect a `gemini-cli` or `agy` session to the Push app for real-time 2-way communication.

### Usage

Run the agent from within a `tmux` session:

```bash
./push --gemini-agent [session-name] [--resume] [--yolo]
# OR
./push --antigravity [session-name] [--resume] [--yolo]
```

*   **`session-name`**: (Optional) A display name for the session. Defaults to the current directory name.
*   **`--resume`**: Resume the latest session.
*   **`--yolo`**: Pass the YOLO flag to the agent for autonomous execution (`-y` for gemini, `--dangerously-skip-permissions` for agy).

### How it Works

1.  **Background Client**: It starts a `push` client in the background (using the same `push` binary) configured with a shared `session_id`.
2.  **2-Way Communication**: 
    *   **Outgoing**: Messages you type in the Push web UI are automatically forwarded to your active `tmux` pane.
    *   **Incoming**: Model responses are captured via hooks (`aftermodel.py`, `afteragent.py`) and sent to the Push app.
3.  **Synchronization**: The script ensures that both the CLI and the web UI are scoped to the same session, providing a unified view of the agent's activity and status.

Signal Integration
------------------

Push supports two-way communication and control via Signal. You can interact with your agent session directly from your Signal app.

### 1. Setting up `signal-cli` Daemon
Push interacts with the native `signal-cli` client running in JSON-RPC daemon mode over HTTP.

To install and register `signal-cli`:
1. Follow the installation guide for [signal-cli](https://github.com/AsamK/signal-cli) for your operating system. On macOS:
   ```bash
   brew install signal-cli
   ```
2. Register your phone number:
   ```bash
   signal-cli -u +YOUR_BOT_NUMBER register
   # Verify the registration with the code received:
   signal-cli -u +YOUR_BOT_NUMBER verify CODE
   ```
3. Start the `signal-cli` JSON-RPC daemon on port `8742`:
   ```bash
   signal-cli-rest-api daemon -signal-cli-config /path/to/config
   # Alternatively, if using a dockerized or custom JSON-RPC helper, ensure the REST API is accessible.
   ```

### 2. Running Push Daemon with Signal
Launch the `push` server with the `--signal-server` and `--signal-address` flags pointing to your registered number:
```bash
./push --signal-server=127.0.0.1:8742 --signal-address=+YOUR_ADMIN_NUMBER
```
*   **`--signal-server`**: Host and port of the running `signal-cli` daemon.
*   **`--signal-address`**: The phone number allowed to control the daemon (only messages received from this sender will be parsed).

### 3. Usage & Commands
Inside your session (either from the web UI or via `--signal` flags), you can control forwarding:

*   **`/signal`**: Activates Signal control for the current session. Responses from the agent will be forwarded to your Signal client, and incoming messages from you will be executed as inputs.
*   **`/signal quiet`**: Activates Signal control and disables normal Web Push notifications for the session (keeping UI and Signal outputs active).
*   **`/signal stop`**: Stops Signal control for the current session.
*   **`/signal +12345678`**: Overrides the target recipient phone number for the session.

### 4. Client CLI Flags
When launching an agent or a tmux client session:
*   Pass `--signal` to automatically enable Signal forwarding on session start and reconnect.
*   Pass `--signal quiet` (or `--signal=quiet`) to immediately enable Signal forwarding and suppress normal push notifications.
```bash
./push --gemini-agent my-session --signal quiet
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

Testing
-------

To run the unit tests, use the following Go command:

```bash
go test -v .
```

To see the test coverage report:

```bash
go test -coverprofile=coverage.out .
go tool cover -func=coverage.out
```
