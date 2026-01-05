#!/bin/bash

# Default values
ADDRESS="127.0.0.1"
PORT="8089"
MESSAGE="Hello from CLI"

# Usage help
usage() {
    echo "Usage: $0 [message] [port] [address]"
    echo "Defaults: message=\"$MESSAGE\", port=$PORT, address=$ADDRESS"
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
    PORT="$2"
fi

if [ ! -z "$3" ]; then
    ADDRESS="$3"
fi

echo "Posting message: \"$MESSAGE\" to http://$ADDRESS:$PORT/interactions"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"$MESSAGE\"}" \
     "http://$ADDRESS:$PORT/interactions"

echo -e "\nDone."

