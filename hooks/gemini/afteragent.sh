#!/bin/zsh

exit
INPUT=$(cat)
echo "${INPUT}" > ~/aa.json
exit
NOTIFICATION_TYPE="$(echo "${INPUT}" | jq -r '.notification_type')"
MESSAGE="$GEMINI_PROJECT_DIR: $(echo "${INPUT}" | jq -r '.message')"

TITLE="Gemini - ${NOTIFICATION_TYPE}"
ADDRESS="127.0.0.1"
PORT="8089"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"$MESSAGE\", \"title\": \"$TITLE\", \"link\": \"\"}" \
     "http://$ADDRESS:$PORT/interactions"

