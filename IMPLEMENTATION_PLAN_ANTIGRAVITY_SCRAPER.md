# Implementation Plan: Antigravity Log Scraper

This plan outlines the steps to implement a log scraper for the Antigravity CLI (`agy`) that provides real-time message updates to the `push` application, mirroring the existing hook-based functionality for `gemini-cli`.

## Phase 1: Research & Discovery
- [x] **Determine Project Hash Logic**: Identify how `agy` determines the project directory name in `~/.gemini/tmp/`. It's likely a hash of the absolute path to the workspace.
- [x] **Log File Mapping**: Confirm the naming convention for session log files (e.g., `session-<ISO_DATE>-<prefix_of_session_id>.jsonl`).

## Phase 2: Scraper Implementation
- [x] **Create `agy_scraper.py`**:
    - [x] Implement file tailing logic for `.jsonl` files.
    - [x] Implement incremental JSON parsing (one object per line).
    - [x] State Management: Track seen message IDs and their latest state.
    - [x] HTTP Client: Send `Interaction` payloads to the `push` backend (`POST /interactions`).
    - [x] Field Mapping:
        - `id` -> `identifier`
        - `type` -> `kind`
        - `content` -> `message` / `detailed_message`
        - `thoughts` -> include in `detailed_message`
        - `status` -> map to `push` status codes (`w` for working, `d` for done, etc.)

## Phase 3: Launcher Integration
- [x] **Update `gemini-agent` Launcher**:
    - [x] Logic to detect/calculate the log file path before starting the agent.
    - [x] Logic to spawn `agy_scraper.py` in the background.
    - [x] Logic to cleanup the scraper process on exit.

## Phase 4: Backend Refinement (if needed)
- [x] **Metadata Inheritance**: Ensure the backend correctly handles updates to interactions that were originally created as "working" placeholders.

## Phase 5: Testing & Verification
- [x] **Unit Tests**: Add tests for the scraper's parsing and mapping logic.
- [x] **Integration Tests**: Run the full stack with `--antigravity` and verify real-time updates.
- [x] **Performance Check**: Ensure the scraper doesn't consume excessive CPU/Memory while tailing.
