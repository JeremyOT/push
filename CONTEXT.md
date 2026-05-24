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
- `-gemini-agent`: Run the embedded agent script with Gemini.
- `-antigravity`: Run the embedded agent script with Antigravity (agy).
- `-resume`: Resume the last agent session.
- `-yolo`: Enable YOLO mode (pass appropriate flags to the agent, e.g. -y for gemini, --dangerously-skip-permissions for agy).

### Commands
The following commands can be sent from the web UI to an active agent session:
- `/run`: Execute the project deployment script (`./deploy.sh`).
- `/restart`: Trigger a fresh restart of the gemini-agent.
- `/restart resume`: Restart the gemini-agent and resume the current session.
- `/new-agent [name]`: Start a new Gemini agent session in a subdirectory named `name` (relative to the current session path).
- `/push register`: Re-register for Web Push notifications (useful if tokens expire).
- `/clear`: Clear the agent's context.
- `/memory reload`: Reload memory and instructions.
- `/compress`: Compress conversation history.

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

## Project Conventions
- The `/run` command is reserved for triggering the project deployment: whenever the user sends `/run`, the agent should execute `./deploy.sh`.
- **Sidebar Session Management:** The "Active" section strictly contains connected sessions. Hierarchical parents of active sessions move to the "Recent" section if they are not themselves connected.
- **Inactive Session Status:** Sessions in the "Recent" list must always show as "passive" with a grey dot, regardless of their last message status.
- **Session Metadata Inheritance:** The backend automatically fills missing `session_path`, `agent`, and `title` for new interactions if a `session_id` is provided, inheriting from the most recent record with that ID.
- **Agent Restarts:** Use `/restart` to trigger a fresh start (new session) or `/restart resume` to restart while keeping the current session. The `gemini-agent` script manages the process lifecycle using UNIX signals (`SIGUSR1` for 101, `SIGUSR2` for 102).

## Recent Changes
- Fixed Antigravity message propagation and UI syncing: aligned `AgyLogLine` struct to support the `step_index`, `status`, `thinking`, and `tool_calls` fields of `transcript_full.jsonl`, mapping step indexes to unique identifiers to prevent message skipping.
- Enhanced tool call integration for Antigravity: mapped model tool execution steps to a dedicated `tool` message kind (preventing truncating console outputs), and added dynamic `ToolPermission` approval cards that automatically resolve/clear when the agent progresses.
- Registered Antigravity as a first-class agent in the client-side configuration with custom fuchsia styling, ring-rendered avatar, and "AG" short representation.
- Improved Antigravity log streaming logic and responsiveness: replaced `bufio.Scanner` with a robust `bufio.Reader` and line accumulator in `main.go`, and reduced polling interval to 100ms for a real-time experience.
- Refined Antigravity user message parsing: the internal Go scraper now extracts and displays only the content within `<USER_REQUEST>` tags for `USER_EXPLICIT` messages.
- Refactored Antigravity (agy) integration to rely exclusively on parsing `transcript_full.jsonl`, eliminating the use of hooks and centralizing messaging logic in the native Go scraper.
- Translated the unified `--yolo` flag to `--dangerously-skip-permissions` when running the Antigravity agent, ensuring consistent autonomous execution behavior.
- Enhanced Antigravity discovery logic: the `gemini-agent` script now dynamically extracts the conversation ID and `appDataDir` from an `agy` runtime log file to locate the correct transcript for scraping.
- Rewrote `agy_scraper.py` in native Go and integrated it into the `push` binary, removing the Python dependency and improving portability.
- Fixed startup failures in `--gemini-agent` mode: implemented PTY provisioning via Python's `pty` module and refactored stdin/stdout forwarding to ensure reliable interactive sessions without `tmux`.
- Removed obsolete `hooks/agy_scraper.py` broken symlink and successfully deployed the latest version of the application using `./deploy.sh`.
- Removed tmux dependency for `--gemini-agent` mode; added a new `pipe` mode to the internal CLI client for transparent message forwarding.
- Improved signal handling and restart logic in `--gemini-agent` mode by centralizing the execution loop in `main.go`.
- Embedded the `gemini-agent` bash script into the `push` binary using `go:embed`.
- Implemented a directory-based tree structure in the sidebar for hierarchical session grouping based on working directory paths.
- Added support for in-place message updates via stable identifiers and explicit `replace: true` parameters.
- Implemented real-time message updates using a `/service` NDJSON streaming endpoint.
- Updated the chat composer keyboard behavior: `Cmd+Return` now sends the message, while `Return` adds a new line.
- Standardized tool permission buttons to "Allow", "Allow Session", and "Deny" with correct numeric index mapping.
- Improved UX by remembering the last selected agent/thread across page refreshes using `localStorage`.
- Enhanced session status logic to automatically set thread status to "working" when the last message is a user message.
- Implemented an animated "three dot" typing/working icon in the web UI.
- Fixed iPhone layout issues by enabling full-screen support and adding safe-area-inset padding.
- Resolved the user message infinite loop and message duplication issues: updated the `tmux` CLI client mode to ignore user messages with non-empty identifiers (scraped from logs), and updated `saveInteraction` to match scraped user messages to existing database records by session ID and content, updating their identifier to prevent duplication.

