#!/bin/zsh

INPUT=$(cat)
#echo $INPUT > ~/stop.json
WD="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .cwd')"
WD="$(basename "${WD}")"

MESSAGE="${WD}: Done"

TITLE="Claude - Done"
ADDRESS="127.0.0.1"
PORT="8089"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"${MESSAGE}\", \"title\": \"$TITLE\", \"link\": \"\", \"detailed_message\":\"${MESSAGE}\"}" \
     "http://$ADDRESS:$PORT/interactions"

