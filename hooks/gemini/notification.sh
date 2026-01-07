#!/bin/zsh

INPUT=$(cat)
#echo "${INPUT}" > ~/note.json
WD="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .cwd')"
MESSAGE="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .message')"
NOTIFICATION_TYPE="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .notification_type')"

MESSAGE="${WD}: ${MESSAGE}"
TITLE="Gemini - ${NOTIFICATION_TYPE}"
ADDRESS="127.0.0.1"
PORT="8089"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"${MESSAGE}\", \"title\": \"$TITLE\", \"link\": \"\"}" \
     "http://$ADDRESS:$PORT/interactions"

