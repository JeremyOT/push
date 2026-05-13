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

## Recent Changes
- Refactored interaction metadata to include an explicit `kind` field (question, approval, agent, status) for robust message rendering, eliminating fragile title-based parsing.
- Updated Gemini CLI hooks (`BeforeTool`, `Notification`) to explicitly report message `kind`, ensuring questions and tool permissions are always correctly identified.
- Enhanced remote tool approval: the Push UI now supports the full range of Gemini CLI options (Allow Once, Allow Session, Allow Forever, Deny) with correct character mappings for remote terminal injection.
- Implemented remote question/options capture: added a `BeforeTool` hook for `ask_user` to forward multiple-choice and text questions to the Push UI.
- Implemented `/push register` command and automated push registration on page load to ensure reliable delivery of Web Push notifications.
- Implemented `/new-agent` command in the web UI (command palette) and backend; allows launching new Gemini agents in specified subdirectories using `tmux new-window`.
- Refined sidebar grouping: the "Active" section now ONLY contains explicitly connected sessions (`active: true`).
- Passive sessions (even if they are parents of active sessions) now correctly move to the "Recent" section, provided they have activity within the last 24 hours.
- Deployed the latest version of the application using `./deploy.sh`.
- Fixed status inconsistency in the sidebar: sessions that are only "active" because of descendant sessions (hierarchical grouping) now correctly show as "passive" (grey dot) if they are not explicitly connected.
- Refined the "ready" agent count in the sidebar footer to only include sessions that are both currently active and in a ready state.
- Started a new Gemini agent session in the `hooks` subdirectory using a dedicated tmux window (`hooks-agent`) and a fresh session ID.
- Built the `push` binary with optimized flags (`-w -s`) to support agent integration.
- Implemented explicit signal handling in `--gemini-agent` mode to ensure the parent `push` process waits for the embedded agent script to clean up after receiving `SIGINT` (Ctrl+C) or `SIGTERM`, preventing orphaned background processes.
- Renamed the agent flag to `--gemini-agent` and updated the implementation to use a temporary script file, ensuring `stdin`, `stdout`, and `stderr` are correctly passed through to the interactive `gemini-cli` session.
- Embedded the `gemini-agent` bash script into the `push` binary using `go:embed` and added a `--gemini-agent` flag to execute it directly.
- Updated `gemini-agent` to support a `PUSH_BINARY` environment variable, ensuring the embedded script uses the correct `push` executable.
- Reduced tmux 'Enter' key delay from 500ms to 200ms for faster interaction while maintaining reliability.
- Improved `tmux` mode reliability by increasing the delay before sending the `Enter` key to 500ms and using the `-l` (literal) flag for `send-keys` to ensure message content is delivered exactly as-is.
- Fixed a bug in `gemini-agent` where `gemini` was being backgrounded, causing it to lose its TTY connection and fail with "no input provided via stdin"; the script now runs `gemini` in the foreground.
- Simplified agent restart logic: replaced UNIX signal-based coordination with a local `.gemini-agent.restart` file. The `push` client now writes the restart mode ("fresh" or "resume") to this file and sends `/exit` to the `gemini-cli` tmux pane, allowing for a cleaner and more robust restart loop in the `gemini-agent` script.
- Updated `gemini-agent` to remove signal traps and implementation of a file-based restart check after the main Gemini process exits.
- Updated `main.go` to handle `/restart` and `/restart resume` by writing to `.gemini-agent.restart` and forwarding `/exit` to tmux.
- Improved UX by remembering the last selected agent/thread across page refreshes using `localStorage`.
- Fixed the "Recent" sidebar section: ensured agents in this section always show as "passive" (grey dot) by supporting status overrides in the hierarchical tree component.
- Fixed a critical regression where messages and agent metadata were missing from the UI due to mismatched SQL `SELECT` and `Scan` calls for the new `session_path` column.
- Implemented a directory-based tree structure in the sidebar: agent sessions are now grouped hierarchically based on their working directory paths.
- Added `--session-path` support to the `push` backend and `gemini-agent` script to track and transmit the session's working directory.
- Refined agent status visualization: agent threads now remain "working" (orange dot) in the sidebar and header after a turn is "done", while inline status notes for "done" messages retain their green dot.
- Implemented a "passive" status (grey dot) for disconnected sessions in the "Recent" sidebar section and agent fleet footer.
- Consolidated "Idle" and "Awaiting" agent statuses into a single "Ready" status with a green dot for consistent visual feedback.
- Updated the sidebar footer to display the count of "ready" agents instead of "awaiting".
- Hardened session grouping and deduplication in the sidebar: implemented strict `session_id` trimming, string-type normalization, and refined thread update logic to prevent duplicate entries for the same session ID.
- Fixed the "Recent" sidebar section: consolidated thread management in `processMessage` to ensure historical sessions are correctly discovered, marked as inactive, and filtered by the 24-hour window.
- Updated the web UI to hide the text input composer on agent threads that are not actively connected to a client.
- Refactored the sidebar to improve flexibility: removed the "Pinned" header, and implemented "Active" (connected) and "Recent" (last 24h) sections.
- Updated `static/chat-app.jsx` to dynamically update thread titles based on the most recent non-system message title (e.g., agent or directory name).
- Enhanced thread state to track raw timestamps for accurate 24-hour filtering in the sidebar.
- Ensured messages without a session ID only appear in the Main Feed and do not create or update worker threads.
- Added brief toast notifications ("Copied to clipboard") that appear when a message is copied via double-tap.
- Implemented "double-tap to copy" functionality for all message bubbles and status notes in the web UI.
- Removed the "stop" button functionality and enabled sending messages at any time, even while the agent is working.
- Fixed session tracking in `gemini-agent` for new sessions: implemented a "start-and-exit" strategy to establish a real session ID before starting the background `push` client and the main interactive session.
- Implemented an animated "three dot" typing/working icon on the agent side of the chat.
- Updated `static/chat-app.jsx` to derive the typing state from the thread status: the indicator shows whenever status is anything other than `ready` or `idle` (and not on the Main Feed).
- Refactored `PushChat` to remove the manual `typing` state in favor of derived `isTyping` and `typingAgent`.
- Updated `Composer` to use the derived `isTyping` state for its stop button visibility.
- Added a new "ready" status (mapped to code `r`) used exclusively by the `afteragent` hook to indicate task completion.
- Updated `static/chat-data.jsx` to include the `ready` status with the same green indicator as `done`.
- Updated `static/chat-app.jsx` to map the backend `r` status code to the frontend `ready` status.
- Updated `main.go` to support displaying `(Ready)` in the CLI service output.
- Fixed a bug where the chat view would not scroll to the bottom when messages were updated in place (e.g., during streaming); memoized `filteredMessages` and updated the scroll effect to trigger on any message content change.
- Updated `gemini-agent` script to prioritize using a local `./push` binary if available, falling back to the system `push` command otherwise.
- Added `/clear`, `/memory reload`, and `/compress` commands to the `CommandPalette` in the web UI.
- Implemented a `/restart` and `/restart resume` command for the `push` client in `tmux` mode; these commands allow the client to signal the parent `gemini-agent` script to restart both the client and the `gemini-cli` session.
- Updated `gemini-agent` to support a restart loop: signal 101 (via `SIGUSR1`) triggers a fresh restart, while signal 102 (via `SIGUSR2`) triggers a restart with session resumption.
- Updated `static/chat-composer.jsx` to include the new commands in the palette items and added support for the `refresh` icon.
- Fixed a bug where the sidebar tree structure was lost after a restart due to missing `session_path` metadata in hook-generated interactions; implemented backend metadata inheritance in `main.go` to ensure all messages in a session share the same path, agent, and title.
- Updated `hooks/gemini/afteragent.py` and `hooks/gemini/aftermodel.py` to explicitly report the full `session_path` (from `cwd`), ensuring hierarchical metadata is preserved across all agent turns.
- Enhanced `session-active` broadcasts to include the session's latest metadata, allowing the frontend to immediately correctly place new active sessions in the tree.
- Fixed a bug where active sessions incorrectly appeared in the "Recent" sidebar section; implemented hierarchical activity tracking to ensure that if a session or any of its descendants is active, the entire tree is moved to the "Active" section.
- Fixed incorrect status dots for inactive sessions; added `statusOverride="passive"` and ensured `AgentMark` and `StatusPill` consistently respect this override to show a grey dot for all recent/inactive threads.
- Fixed a bug where system messages (like `session-active`) with ID 0 were ignored by the frontend due to a strict `lastMsgId` check; the UI now processes all messages with ID 0 to ensure real-time status and activity updates are reflected immediately.
- Improved `AgentMark` in the sidebar to include a status dot, providing better visual feedback for session states in the thread list.
- Fixed several `main_test.go` failures by updating `runCliClient` calls to match the current function signature.
## Project Conventions
- The `/run` command is reserved for triggering the project deployment: whenever the user sends `/run`, the agent should execute `./deploy.sh`.
- **Sidebar Session Management:** The "Active" section strictly contains connected sessions. Hierarchical parents of active sessions move to the "Recent" section if they are not themselves connected.
- **Inactive Session Status:** Sessions in the "Recent" list must always show as "passive" with a grey dot, regardless of their last message status.
- **Session Metadata Inheritance:** The backend automatically fills missing `session_path`, `agent`, and `title` for new interactions if a `session_id` is provided, inheriting from the most recent record with that ID.
- **Agent Restarts:** Use `/restart` to trigger a fresh start (new session) or `/restart resume` to restart while keeping the current session. The `gemini-agent` script manages the process lifecycle using UNIX signals (`SIGUSR1` for 101, `SIGUSR2` for 102).

## Recent Changes
- Fixed a race condition and message ordering bug in the frontend: `fetchInitial` now processes historical messages in a sorted, deduplicated order, and ensures the latest context for the active session is always fetched upon refresh.
- Optimized `QuestionCard` to send single-character terminal-compatible responses (`y`/`n`, `1`, `2`, etc.) to ensure reliable interaction with CLI-based agents.
- Resolved redundant tool permission prompts by suppressing "ToolPermission" notifications in `hooks/gemini/notification.py` when the calling tool is `ask_user`.
- Fixed a backend bug in `main.go` where the message `kind` field was lost during certain database `INSERT` operations.
- Added cache-busting version parameters to frontend script tags in `index.html`.
- Fixed a bug where regular questions (from `ask_user`) were incorrectly triggering tool permission dialogs (Allow Once, Deny, etc.) by consolidating and prioritizing message `kind` detection in the frontend.
- Successfully started the `hooks-agent` session in the `hooks` subdirectory on the local machine (`darwin`), verifying it reached the interactive prompt.
- Verified the `beforetool` hook by asking questions, which were intercepted and posted as interactions to the local `push` server.
- Implemented "stop" functionality for Gemini agent: the UI now replaces the send button with a stop button while the agent is working, and the `Escape` key can be used to interrupt the agent via `tmux send-keys`.
- Refined the Markdown styling to remove excessive vertical padding in message bubbles.
- Implemented full Markdown support for received messages in the web UI using the `marked` library, including support for headings, lists, code blocks, and tables.
- Removed `/new thread` and `/search` commands from the command palette.
- Streamlined the sidebar by removing the search box and "New task" button.
- Enhanced the sidebar footer: agent icons are now interactive and select the corresponding thread when clicked.
- Improved `aftermodel` hook to skip sending empty notifications (avoiding "prefix:"-only messages) and ensuring final status updates don't accidentally clear accumulated message content.
- Improved session activity tracking: the CLI client now re-registers with the server immediately upon establishing a successful connection (including reconnections), ensuring active sessions are correctly displayed in the web UI even after a server restart.
- Restored `/new-agent` command to use `tmux new-window`, allowing it to spawn new agent sessions in dedicated windows as originally intended.
- Reverted `gemini-agent` script to run the background push client in the same pane (backgrounded) to ensure it correctly detects the active pane and receives shared `stdin` input for 2-way communication.
- Modified `main.go` to support spawning new agent sessions via `tmux new-window` from both the web UI and the CLI client.
- Improved `/new-agent` command UX: the initiating agent now sends a confirmation message with a "ready" status (`r`) upon successful agent creation, ensuring the UI transitions out of the "working" state and provides clear feedback.
- Fixed a bug where the `/new-agent` command was double-creating agents by coordinating its handling between the server and CLI clients:
    - The server now handles `/new-agent` exclusively for global messages (Main Feed).
    - CLI clients now handle `/new-agent` exclusively for messages matching their specific `session_id`.
- Updated `runCliClient` to support passing through the `--yolo` flag to newly created agents.
- Refined the `/service` stream filtering to ensure consistent message delivery while preventing redundant command execution.
- Fixed a bug where message statuses and content were not properly updating inline when upserted via the `aftermodel` hook; set `replace: true` in the hook and improved backend field merging.
- Fixed sidebar status display: the "ready" status now appears for worker threads, while all status indicators are hidden for the "Main Feed" thread.
- Fixed top bar status display in `ChatHeader`: status indicators are now hidden for the "Main Feed" thread and dynamically update for worker threads.
- Enhanced session status logic to automatically set the thread status to "working" when the last message in a session is a user message, assuming the agent is preparing a response.
- Fixed a bug where the thread status in the sidebar was not updating when a new message arrived.
- Fixed stale active session list on reload by sending an immediate heartbeat with currently active session IDs upon connection to the `/service` stream.
- Fixed sidebar timestamps and snippets not updating by ensuring `processMessage` updates the corresponding thread's state when a new message arrives.
- Improved session activity tracking in the frontend by differentiating between historical and real-time messages.
- Added `--yolo` flag to `gemini-agent` which passes `-y` to the underlying `gemini` CLI.
- Fixed a bug in `gemini-agent` where the background `push` client would exit immediately due to incorrect terminal detection for redirected `stdin` (`/dev/null`).
- Implemented robust terminal detection in `main.go` using `ioctl` (`TIOCGWINSZ`) to accurately distinguish between a real TTY and background/redirected input.
- Updated `gemini-agent` to use the local `./push` binary and generic `gemini` commands for better portability.
- Fixed a bug in the CLI client where messages sent in `text` mode were missing `session_id`, `agent`, and `title` metadata, leading to incorrect attribution in the web UI.
- Enhanced `runCliClient` to consistently apply session and agent metadata to all outgoing messages, including those sent via `stdin`.
- Added `TestRunCliClientMetadata` to verify end-to-end CLI session registration and message attribution.
- Added `TestUserMessagePushSuppression` to verify that `is_user: true` messages correctly bypass push notification logic.
- Renamed the "Recent" tab to "Active" in the sidebar and implemented real-time session activity tracking; only sessions with an actively connected CLI client are now displayed.
- Added automatic removal of inactive sessions from the web UI, while ensuring the "Main Feed" remains pinned and visible.
- Enhanced the `/service` endpoint and heartbeat mechanism to broadcast session activity status (`session-active`, `session-inactive`) and sync active session lists across clients.
- Updated `gemini-agent` script to support `--resume`, allowing it to continue the last session and reuse its session ID for both `gemini-cli` and the background `push` client.
- Added `gemini-agent` script to launch `gemini-cli` and a background `push` tmux client synchronized by a shared session ID.
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