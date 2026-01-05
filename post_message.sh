#!/bin/bash

# Default values
ADDRESS="127.0.0.1"
PORT="8089"
MESSAGE="Hello from CLI"
TITLE=""

# Usage help
usage() {
    echo "Usage: $0 [message] [title] [port] [address]"
    echo "Defaults: message=\"$MESSAGE\", title=\"$TITLE\", port=$PORT, address=$ADDRESS"
    exit 1
}

# Parse arguments
if [ "$1" == "-h" ] || [ "$1" == "--help" ]; then
    usage
fi

if [ ! -z "$1" ]; then
    MESSAGE="$1"
fi

if [ ! -z "$2" ]; then
    TITLE="$2"
fi

if [ ! -z "$3" ]; then
    PORT="$3"
fi

if [ ! -z "$4" ]; then
    ADDRESS="$4"
fi

echo "Posting message: \"$MESSAGE\" with title: \"$TITLE\" to http://$ADDRESS:$PORT/interactions"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"$MESSAGE\", \"title\": \"$TITLE\"}" \
     "http://$ADDRESS:$PORT/interactions"

echo -e "\nDone."