Push
====

Push is a lightweight one way notification system written in Go that serves a webpage over http, displaying a local database of interactions/messages as a one way chat in an imessage style interface.

It uses the Push API (https://developer.mozilla.org/en-US/docs/Web/API/Push_API) to notify consumers of new interactions when offline, and dynamically updates the chat view as new interactions are posted.

Usage
-----

Run the server as follows:

./push --address=BIND_ADDRESS --port=PORT --database=DATABASE

BIND_ADDRESS defaults to 127.0.0.1
PORT defaults to 8089
DATABASE defaults to "./push.sqlite"

Post new interactions by sending JSON POST requests to http://BIND_ADDRESS:PORT/interactions with the body:

```json
{
  "title": "Optional Title",
  "message": "The interaction text",
  "detailed_message": "Optional longer message",
  "link": "https://example.com/optional-link"
}
```

View messages by visiting http://BIND_ADDRESS:PORT.

Services & Advanced Usage
-------------------------

Push provides several ways to integrate with other services and tools.

### 1. Simple Messaging (The `/interactions` Endpoint)

The easiest way to send a message is using a standard JSON POST request, as seen in `post_message.sh`:

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

The `push` binary includes a built-in CLI client to interact with a running Push server:

```bash
./push --address=localhost:8089 --cli-service=[MODE]
```

**Modes:**
*   `text` (Default): An interactive chat-like interface.
*   `json`: Outputs each received message as a JSON object on a new line. It also expects JSON input for sending messages. Ideal for piping to tools like `jq`.
*   `jsonr`: Interactive mode like `text` but optimized for certain terminal environments.
*   `tmux`: Forwards user messages received from the web app to a specified tmux pane. Requires `--tmux-target`.

### 4. Tmux Integration

The `tmux` mode allows you to forward messages directly into a tmux pane. This is useful for remote command execution or providing input to a running process from the Push web interface.

```bash
./push --address=localhost:8089 --cli-service=tmux --tmux-target="mysession:mywindow.1"
```

When started, it notifies the web app that it's forwarding to the specified pane. When it exits, it sends a "no longer forwarding" message.

Implementation
--------------

The implementation is a single binary written in Go with embedded html/javascript/css using Golang's embed package. Interactions are stored in a local sqlite database.
