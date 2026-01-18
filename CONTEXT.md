# Context

This file documents the current state and context of the `push` application.

## Overview
`push` is a lightweight notification system that serves a webpage displaying a feed of messages. It supports Web Push notifications.

## CLI Options
The application has been updated to support a client mode for sending messages directly from the CLI.

### Flags
- `-address`: Bind address (default: 127.0.0.1)
- `-port`: Port (default: 8089)
- `-database`: Path to SQLite database (default: ./push.sqlite)
- `-hostname`: Hostname for push notifications
- `-reset-vapid`: Reset VAPID keys
- `-m`: Message content. Presence of this flag triggers client mode.
- `-t`: Title of the message (optional, used in client mode).

## Recent Changes
- Added `-m` and `-t` flags to support sending messages via CLI without using `curl`.
