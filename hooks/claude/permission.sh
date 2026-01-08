#!/bin/zsh

INPUT=$(cat)
#echo "${INPUT}" > ~/permission.json
WD="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .cwd')"
WD="$(basename "${WD}")"
TOOL_NAME="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .tool_name')"
TOOL_INPUT="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .tool_input')"
cat "${WD}" > ~/pmwd.json
MESSAGE="${WD}: Permission Request ${TOOL_NAME}"
#DETAILED_MESSAGE="${WD}: Permission Request ${TOOL_NAME} ${TOOL_INPUT}"
DETAILED_MESSAGE="${WD}: Permission Request ${TOOL_NAME}"
cat "${MESSAGE}" > ~/pm.json
TITLE="Claude - Permission Request"
ADDRESS="127.0.0.1"
PORT="8089"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"${MESSAGE}\", \"title\": \"$TITLE\", \"link\": \"\", \"detailed_message\": \"${DETAILED_MESSAGE}\"}" \
     "http://$ADDRESS:$PORT/interactions"

