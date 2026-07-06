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
- Suppressed push notifications for tool permission approvals when running the agent in `--yolo` mode by setting the approval interaction's `quiet` property to match the `yolo` flag.
- Transitioned thread status back to "ready" immediately when the agent finishes execution (finalized `PLANNER_RESPONSE` with no tool calls) to stop the typing indicator and unlock the composer input.
- Added a robust `cleanup()` function and `EXIT` trap to the embedded `gemini-agent` script to ensure that backgrounded push client processes and temporary log scrapers/files are properly terminated and deleted when the agent exits or is interrupted, preventing process proliferation. Added the `TestGeminiAgentScriptCleanup` unit test to verify this.
- Fixed conversation message ordering and session isolation for Antigravity (agy) sessions: scoped the identifier check-if-exists query in `saveInteraction` by `session_id` to prevent messages from different sessions with the same step-index identifier from overwriting each other, and updated the message sorting logic in the React web frontend (`chat-app.jsx`) to sort by numeric transcript step index (`identifier`) for Antigravity sessions instead of database autoincrement `id` (as backfilled prior steps get higher database IDs than initial user messages). Added the `TestAgySessionIsolation` unit test.
- Added support for translating the `--resume` flag to `--continue` when used with `--antigravity` (`agy`), registering the `--continue` CLI flag in the `push` binary, translating positional/extra resume arguments, and mapping them inside the embedded `gemini-agent` script. Added unit tests for flag/argument translation and the embedded script alias check.
- Fixed message ordering for non-indexed system messages (e.g. "Registered session" and "Now forwarding responses") in Antigravity threads by dynamically interleaving them relative to step-indexed messages using their insertion/database IDs.
- Fixed a client-side update propagation bug in `static/chat-app.jsx` where incoming message updates with matching database IDs but new identifiers (e.g., when a user message is matched by the log scraper and gets assigned a step index) were ignored in the React state. Messages are now correctly updated in-place when their database IDs match, ensuring they dynamically sort into their correct interleaved position without requiring a page refresh.
- Suppressed the generation and sending of "ToolPermission" approval cards altogether when running the agent in `--yolo` mode, preventing UI noise since the agent automatically approves these actions. Fixed a potential nil pointer dereference crash in the scraper loop if the watched log file is deleted, and added the `TestAgyScraperYolo` unit test to verify suppression.
- Aligned Antigravity push session IDs with the actual CLI conversation ID by delaying push client launch and registration until the conversation ID is parsed from the log file. This enables `--resume` restarts to reuse the exact same push session and correctly preserve message history in the UI. Added dynamic PID tracking file support in the launcher script to ensure correct background process cleanup.
- Fixed a registration deadlock for fresh Antigravity runs: the script now launches the push client immediately using a temporary UUID to allow Web UI registrations, and then uses a new `/rename-session` API endpoint to seamlessly rename the session in the database and restart the push client under the real conversation ID once parsed. Added the `TestRenameSession` unit test.
- Added CLI argument normalization for em-dash (`—`) and en-dash (`–`) characters to standard hyphens (`--`), correcting macOS auto-correction behavior when executing flags (e.g. `—antigravity` and `—yolo`). Added the `TestNormalizeArgs` unit test to verify this.
- Added client-side normalization in `static/chat-composer.jsx` to dynamically map em-dashes (`—`) and en-dashes (`–`) back to standard double-hyphens (`--`) upon sending a message to automatically correct macOS auto-correction/substitution issues while keeping standard spellcheck and autocapitalize features enabled.
- Fixed a bash syntax error in the embedded `gemini-agent` launcher script (changed `do` back to `then` on line 170 in an `if` block) that broke fresh session runs.
- Enhanced link styling and readability in the chat UI: added a `link` color token to theme configurations (#58a6ff for dark mode and #0969da for light mode) and styled `.markdown-body a` elements accordingly, eliminating low-contrast default blue links on dark backgrounds.
- Modernized the application color palettes in `static/chat-theme.jsx`, transitioning dark mode to a premium deep slate-navy theme (#0b0f19 background, #111420 panels, and #171c2c hover states) with modern indigo accents (#6366f1/#4f46e5). Refined scrollbars to be thinner and translucent, and updated inline code / pre block container styles with cleaner backgrounds and borders.
- Added support for parsing `ask_question` tool calls in `main.go`, mapping them to UI-native question cards (interactive choice/options blocks) so users can answer agent queries directly from the Web UI. Added the `TestAgyScraperQuestion` unit test.
- Fixed an issue where Antigravity sessions registered with a Tmux icon initially and after server restarts, and where the temporary session ID registration caused duplicate sidebar listings. Specifically, updated `main.go`'s `runCliClient` to use the dynamically resolved `agent` instead of the hardcoded `"tmux"` when sending forwarding and status messages, and modified `gemini-agent` to kill the startup push client and wait for its exit status message to finish before triggering the `/rename-session` database update. Added the `TestRunCliClientTmuxAgent` unit test.
- Fixed an issue where the Antigravity session initialized in a "busy" state and stayed locked in the UI until the user hit "stop". Specifically, updated the CLI client registration status on reconnect in `main.go` from `"d"` (done) to `"r"` (ready), and updated the frontend `isTyping` condition in `static/chat-app.jsx` to treat the `'done'` status as a non-typing/ready state so that completed agent states do not block the composer. Added assertions in `TestRunCliClientMetadata` to verify the registration status.
- Fixed an issue where terminating the antigravity session directly via the terminal left a stranded `--cli-service` session background process behind. Specifically, unified `PUSH_PID_FILE` in `gemini-agent` to a constant path `/tmp/push-client-$$.pid` using the parent shell PID (which is identical and visible in both parent and background subshells), simplified the background subshell to directly write to this PID file instead of using dynamic temporary UUIDs/filenames that the parent could not read, and updated the `cleanup()` function to use a `kill -0` loop to wait for process termination. Added assertions in `TestGeminiAgentScriptCleanup` to verify the new tracking and cleanup behavior.
- Fixed a UI/state synchronization issue during fresh session initialization (where the initial temporary UUID session would appear to terminate and be replaced by the final Antigravity conversation ID session). Specifically, updated the `/rename-session` API endpoint in `main.go` to update the server's `activeSessions` map and broadcast a `session-rename` event, and updated the `processMessage` function in `static/chat-app.jsx` to process the event in real-time, mapping existing messages, merging sidebar threads, and updating the active thread selection state seamlessly. Updated the `TestRenameSession` unit test.
- Fixed a process hanging issue on exit where the background `--cli-service` push client would ignore SIGTERM/SIGINT signals and require `kill -9` to terminate. Specifically, added a fallback timeout mechanism (1 second) in the signal handling goroutine of the CLI service to force-exit the process if it fails to shut down gracefully. Also added an active connection close listener in the `runCliClient` streaming receiver loop, spawning a goroutine that explicitly closes `resp.Body` when the context is cancelled to ensure `json.Decoder.Decode` returns immediately and does not deadlock.
- Avoided infinite hangs on exit inside the `gemini-agent` launcher script by replacing all unbounded `kill -0` check loops with bounded timeout loops (maximum 1 second / 10 iterations) and adding a fallback `kill -9` (SIGKILL) signal if processes do not exit gracefully. Reordered the cleanup sequence to terminate the scraper process first so that the push client becomes reparented to PID 1 (allowing it to be automatically reaped by the OS rather than staying as a zombie of the running parent script). Updated the `TestGeminiAgentScriptCleanup` unit test to verify the presence of the loop limit and SIGKILL fallback.
- Implemented a question capturing strategy for Antigravity sessions using `tmux capture-pane -p` to scan the active tmux pane when the log scraper is waiting/idle. The scraper parses terminal question prompts, converts them into interactive question card payloads for the Web UI, and resolves the questions automatically once they are no longer visible in tmux. Added the `TestParsePaneQuestion` unit test to verify parsing.
- Fixed a rendering issue with interactive question cards caused by `mergeStrings` concatenating JSON payloads in the `detailed_message` database column upon message updates, resulting in invalid JSON. Questions of kind `question` are now excluded from string merging.
- Fixed a scrollback parsing issue where the `checkTmuxQuestion` scraper would capture and parse stale/scrolled-up questions from the tmux pane history during subsequent agent steps. Constrained `parsePaneQuestion` to verify that the navigation prompt is near the bottom of the visible terminal output.
- Implemented inline composer input support for the write-in option in `static/chat-app.jsx`: when the last choice option is exactly "Write-in...", selecting it re-enables the composer input bar (hiding the "stop" button), allowing the user to type their custom response directly in the chat composer. Deferred card resolution (`setDecisions`) until the user submits their response so the question card remains active while they are typing. Added robust exact matching for the "Write-in..." option (case-insensitive and trimmed). Upon submission, the UI sends the option index (suppressing Enter), waits 500ms, and sends the user's custom input followed by Enter.
- Suppressed sending of trailing `Enter` key when selecting choice and yes/no options in tmux service mode by introducing a `choice` kind indicator, preventing premature confirmation and skipping of write-in prompts in terminal interactive interfaces. Added `TestRunCliClientTmuxChoice` to verify.
- Suppressed informational console log outputs printed to stdout and stderr during normal startup, shutdown, and restart operations. Suppressed logging in the embedded launcher script (`gemini-agent`) and the log scraper (`main.go`). Added the `TestGeminiAgentScriptNoInfoLogging` unit test to verify that normal operational messages are not printed.
- Suppressed connection failure, stream close, send error, and service notification logs on exit and during tmux background client operation. Updated the `gemini-agent` cleanup trap to disown background jobs before termination to suppress shell job status messages on exit.
- Suppressed push notifications for historical log contents on startup by introducing a `catchingUp` flag in the log scraper that sets `Quiet: true` for all messages processed prior to the first EOF. Deduplicated consecutive duplicate "Ready" status messages in `saveInteraction` to avoid displaying repeating ready status lines. Added `TestSaveInteractionConsecutiveReady` and verified catch-up quiet properties in `TestAgyScraper`.
- Enhanced decided question card rendering in `static/chat-messages.jsx` to display the option text or write-in responses alongside the chosen index, e.g. "Answered: 4. Option label (custom write-in text)", and updated the decided status calculation to check backend database state. Reconstructed decisions dynamically from the message history using a `useMemo` hook in `static/chat-app.jsx` to ensure they persist across page refreshes.
- Fixed a shell syntax error in [gemini-agent](file:///Users/jeremyot/dev/push/gemini-agent) where `local` was incorrectly used to declare a variable inside a background subshell instead of a function, resolving the "local: can only be used in a function" error in log output.
- Unified tool permission dialogs with the native question card mechanism by updating [main.go](file:///Users/jeremyot/dev/push/main.go) to treat tool permissions as questions. Enhanced the tmux pane scraper in [parsePaneQuestion](file:///Users/jeremyot/dev/push/main.go#L2331) to parse multi-line question blocks (supporting Action, Target, and Reason detail lines) and identify tool permissions. Updated [QuestionCard](file:///Users/jeremyot/dev/push/static/chat-messages.jsx#L308) in [chat-messages.jsx](file:///Users/jeremyot/dev/push/static/chat-messages.jsx) to display a "Tool Permission" header and the [IconCommand](file:///Users/jeremyot/dev/push/static/chat-icons.jsx#L13) icon when displaying tool permission prompts. Added [TestParsePaneToolPermission](file:///Users/jeremyot/dev/push/main_test.go#L2147) and updated existing scraper tests in [main_test.go](file:///Users/jeremyot/dev/push/main_test.go).
- Fixed the tool permission denial workflow in the Web UI: when the last choice option (typically "Deny permission") of a Tool Permission question card is selected, the application now automatically reverts the thread status to "ready" immediately and prevents it from being incorrectly overwritten to "working" upon receipt of the user's choice broadcast message. Completely removed parsing of tool permissions from the session log scraper (`main.go`), relying exclusively on live tmux pane parsing to list prompt options dynamically rather than using predefined inputs. Added the `TestStaticAssetsContainsToolDenyLogic` unit test to verify the implementation.
- Fixed parsing of tool permission dialogs from the tmux terminal pane by relaxing the navigation prompt detection to match "Navigate" (supporting prompts without "Select" and "Skip" keys, such as those in tool permission prompts) and ensuring empty lines within multi-line permission/question blocks do not terminate parsing prematurely unless a terminal separator/prompt is hit. Also enhanced tool permission identification by matching permission options like "allow access" and "deny access". Added the `TestParsePaneToolPermissionNewFormat` unit test to verify.
- Implemented real-time detection of token quota exceeded errors in the tmux pane scraper (`main.go`). When an "Individual quota reached" message is parsed, it sends a formatted informational markdown warning (containing any associated "Error ID:") to the frontend as an agent message with status `r` (ready), which prevents the application from blocking user inputs and keeps the session in the ready state. Added the `TestParsePaneQuotaReached` unit test to verify parsing.
- Added support for parsing and responding to bracket-based CLI experience feedback prompts (e.g. `[1] Good  [2] Fine  [3] Bad  [0] Skip`) in the tmux pane scraper (`main.go`). Updated `parsePaneQuestion` to return option labels alongside their corresponding prompt keys, updated `checkTmuxQuestion` to include custom values in `UIOption` payloads, updated the frontend (`chat-app.jsx` and `chat-messages.jsx`) to map button clicks to these custom values, and added the `TestParsePaneCliExperience` unit test.
- Implemented automatic image scraping, downscaling, embedding, and horizontal carousel rendering. When a message (message or detailed_message) contains local image file names, relative/absolute paths, or public URLs (e.g. in text, markdown, or HTML format), the system extracts them. For public URLs, the database stores the direct URL without embedding the payload, allowing the client to load and render the image directly. For local images, the backend reads, downscales (preserving aspect ratio, up to 1024px maximum dimension if >256KB), and embeds them as base64 PNG data URLs in the database. Added support in the frontend React application (`chat-app.jsx` and `chat-messages.jsx`) to map these images and render them at the end of user and agent messages in a horizontally scrollable carousel. Integrated a lightbox/modal viewer to view full resolution images on click. Added the `TestImageScraper` unit test.
