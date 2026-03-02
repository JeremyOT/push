# Context

This file documents the current state and context of the `push` application.

## Overview
`push` is a lightweight notification system that serves a webpage displaying a feed of messages. It supports Web Push notifications.

## CLI Options
The application has been updated to support a client mode for sending messages directly from the CLI.

### Flags
- `-address`: Address and port to listen on (default: 127.0.0.1:8089)
- `-database`: Path to SQLite database (default: ./push.sqlite)
- `-hostname`: Hostname for push notifications
- `-reset-vapid`: Reset VAPID keys
- `-m`: Message content. Presence of this flag triggers client mode.
- `-t`: Title of the message (optional, used in client mode).
- `-application-title`: Custom title for the web application (replaces "Push").
- `-icon`: Path to a PNG file to replace the application's icons (automatically resizes to required sizes).
- `-static-output`: Output directory to export the fully rendered static web app content.
- `-interactive`: Enable interactive mode to allow sending messages from the web app.
- `-cli-service`: Enable interactive CLI mode (modes: `text`, `json`, `jsonr`, `tmux`).
    - `text`: Standard text input/formatted output.
    - `json`: NDJSON input and output.
    - `jsonr`: Text input and NDJSON output.
    - `tmux`: Forwards user messages to a specified tmux pane. Can optionally specify a client ID using `tmux:client_id` to only process messages prefixed with `client_id ` or `client_id: `.
- `-tmux-target`: Target tmux pane for `tmux` mode (e.g., session:window.pane).

## API Endpoints
- `GET /interactions`: Fetch messages (supports `after`, `before`, and `limit` parameters).
- `POST /interactions`: Send a new message.
- `GET /service`: Stream user messages as NDJSON. Supports `timestamp` query parameter.
- `POST /service`: Stream a set of messages as NDJSON to be sent.
- `GET /vapid-public-key`: Get the VAPID public key for push subscriptions.
- `POST /subscribe`: Register a new push subscription.

## Build Instructions
To build the binary, especially on macOS to avoid linker warnings:
```bash
go build -ldflags="-w -s" -o push main.go
```

## Recent Changes
- Fixed Gemini hooks (`afteragent.py`, `aftermodel.py`) to properly extract full model responses and enabled the `afteragent` hook.
- Suppressed push notifications for user-sent messages (`is_user: true`) while maintaining immediate broadcast.
- Broadened `/service` stream to include all messages (both user and service) for real-time updates across all clients.
- Fixed push notification delivery by reverting to standard `webpush-go` VAPID handling and removing custom `VAPIDTransport`.
- Resolved `BadJwtToken` and `P256 point not on curve` errors occurring on Go 1.25.
- Standardized VAPID expiration to 45 minutes for improved Apple Push Service (APNs) compatibility.
- Added modes (`text`, `json`, `jsonr`) to `--cli-service` for flexible input/output.
- Added `--cli-service` flag for real-time interactive CLI chat.
- Added `/service` streaming endpoint for NDJSON-based real-time interaction.
- Added `-interactive` flag to enable sending messages from the web client.
- Added `-application-title` and `-icon` flags for web app customization.
- Added `-static-output` flag to export the web app with all customizations.
- Added `-m` and `-t` flags to support sending messages via CLI.
- Added `tmux` mode to `--cli-service` for forwarding user messages to a tmux pane.
- Added `-tmux-target` flag to specify the destination tmux pane.
- Improved `tmux` mode reliability by splitting `send-keys` and adding a 100ms delay before `Enter`.
- Added start/exit notification messages for `tmux` mode to inform web clients of the forwarding state.
- Added reconnection logic with exponential backoff for `--cli-service` to handle connection losses gracefully.
- Improved `--cli-service` reliability by tracking message timestamps to avoid data loss during reconnection.
- Redirected all `--cli-service` connection logs and errors to `stderr` for better piping support.
- Added `tmux:client_id` format to `--cli-service` to filter and strip prefixes from user messages.
- Updated `README.md` with comprehensive usage instructions, flag lists, and feature documentation.
- Improved `tmux` mode robustness by ensuring the process continues running as a receiver even if `stdin` is closed.
- Enhanced CLI client error logging to provide more details when failing to notify the service.
- Added signal handling to CLI client for graceful termination logging.
- Added explicit `tmux` availability check and detailed error reporting for `tmux` command failures.
- Added 5s timeout to initial CLI client notification and 100ms delay for graceful exit messages.
- Added small delay to receiver goroutine startup for improved synchronization with sender.
- Added comprehensive unit tests in `main_test.go` (covering Broadcaster, database, handlers, static content, and CLI logic).
- Refactored `runCliClient` to support `io.Reader`/`io.Writer` and `context.Context` for improved testability.
- Extracted CLI message-sending logic into a separate `sendMessage` function for independent verification.
- Implemented `TestRunCliClient` and `TestRunCliClientModes` to verify end-to-end behavior of the CLI service (text, json, jsonr modes).
- Improved overall code coverage to 56.5% and updated `README.md` with testing instructions.
- Enhanced `tmux` mode to only block on EOF if `stdin` is not a terminal, restoring normal interactive quit behavior.