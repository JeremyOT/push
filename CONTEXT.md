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
- Restricted text input in the web UI: the composer is now hidden on the "Main Feed" and only appears when an agent-specific thread is selected.
- Fixed a UI crash caused by a `ReferenceError` (temporal dead zone) when accessing `filteredMessages` before its initialization.
- Added automatic scroll-to-bottom when switching between threads or the main feed in the web UI.
- Implemented per-agent filtering in the web UI using `session_id`.
 The main feed continues to show all messages, while specific agent threads filter by `session_id`.
- Added dynamic thread creation in the frontend; agents registered via the CLI now appear automatically in the sidebar.
- Added `--session-id`, `--session-name`, and `--model` flags to the CLI service for better agent attribution and session-scoped interactions.
- Updated `hooks/gemini/afteragent.py` and `hooks/gemini/aftermodel.py` to extract and include `session_id` from Gemini CLI event data.
- Enhanced `/service` endpoint with `session_id` filtering, ensuring clients only receive relevant messages when a session is active.
- Enhanced in-place message updates to merge and preserve fields (title, link, status, agent, session_id) during incremental updates by identifier.
- Updated `hooks/gemini/afteragent.py` to use the new `status` and `agent` fields, ensuring consistent status reporting across all Gemini hooks.
- Refactored message handling to use explicit `status` and `agent` fields instead of parsing the title in both the web interface and CLI client.
- Added `status` (w/d) and `agent` fields to the `interactions` table and `Interaction` struct for more robust message attribution and state tracking.
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
- Updated `hooks/gemini/afteragent.py` to send "quiet" notifications, suppressing push alerts for automated agent status updates.
- Added "quiet" mode for interactions: when `quiet: true`, messages are broadcast to clients but skip push notifications.
- Updated database schema and migrations to include the `quiet` column in the `interactions` table.
- Enhanced `handleInteractions` and `handleService` to support the `quiet` field in both GET and POST requests.
- Added unit tests to verify `quiet` field persistence and push notification suppression.
- Enhanced `tmux` mode to only block on EOF if `stdin` is not a terminal, restoring normal interactive quit behavior.
- Updated the web interface with a new React-based design from "AI Chat.zip", including support for multiple message types (agent, user, tool, approval) and dark/light modes.
- Implemented real-time message updates using the `/service` NDJSON streaming endpoint with a fallback polling mechanism for improved reliability and responsiveness.
- Added support for titles and links in the new message bubbles.
- Maintained compatibility with `-application-title`, `-icon`, and `-interactive` flags by adapting the new design to the backend's injection patterns.
- Renamed the default message sender from "Gemini" to "Remote" with a neutral color scheme and updated all message mapping logic.
- Fixed iPhone layout issues by enabling full-screen support with `viewport-fit=cover` and adding `safe-area-inset` padding to the chat header and composer, replacing the previous mock `IOSDevice` frame.
- Enabled dynamic app icon updating in the web interface by using `icon.svg` as the source, allowing the backend to replace it with a custom icon when the `--icon` flag is used.
- Added dynamic agent detection in the web interface; messages with titles formatted as "AgentName - Status" (e.g., "Gemini - Done" or "Claude - permission_prompt") are now correctly attributed to that agent with its corresponding design-specified colors and icons, defaulting to "Remote".
- Implementation of dynamic in-place message updates via an optional user-supplied `identifier`, with full support in the Go backend and React frontend.
- Messages with a matched `identifier` now append new text by default; explicit replacement is supported via a `replace: true` parameter.
- Updated `hooks/gemini/aftermodel.py` to use `replace: true` when supplying message updates, ensuring full turn content is correctly displayed.
- Updated `hooks/gemini/aftermodel.py` to supply a stable message identifier derived from the hash of the `llm_request` field (filtered to only include contents up until the most recent "user" message), enabling in-place updates of model responses during a single turn.
- Refined agent status display in the web interface and hooks to show the "Done" status when appropriate.
- Fixed a regression where user messages were not appearing in the feed by correcting the `mapMessage` logic and refining state update conditions to properly handle both new messages and in-place updates.