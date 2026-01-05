#!/bin/bash

# Default values
ADDRESS="127.0.0.1"
PORT="8089"
MESSAGE="Hello from CLI"
TITLE=""
LINK=""

# Usage help
usage() {
    echo "Usage: $0 [message] [title] [link] [port] [address]"
    echo "Defaults: message=\"$MESSAGE\", title=\"$TITLE\", link=\"$LINK\", port=$PORT, address=$ADDRESS"
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
    LINK="$3"
fi

if [ ! -z "$4" ]; then
    PORT="$4"
fi

if [ ! -z "$5" ]; then
    ADDRESS="$5"
fi

echo "Posting message: \"$MESSAGE\" with title: \"$TITLE\" and link: \"$LINK\" to http://$ADDRESS:$PORT/interactions"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"$MESSAGE\", \"title\": \"$TITLE\", \"link\": \"$LINK\"}" \
     "http://$ADDRESS:$PORT/interactions"

echo -e "\nDone."
