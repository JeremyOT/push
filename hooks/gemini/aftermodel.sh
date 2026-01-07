#!/bin/zsh

INPUT=$(cat)
WD="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .cwd')"
MESSAGE="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .llm_response.text')"
FINISH_REASON="$(echo "${INPUT}" | jq -Rnr '[inputs] | join("\\n") | fromjson | .llm_response.candidates[0].finishReason')"

if [[ -z "${FINISH_REASON}" ]] || [[ "${FINISH_REASON}" == "null" ]] || [[ "${MESSAGE}" == "null" ]]; then
  exit
fi
NOTIFICATION_TYPE=Done
MESSAGE="${WD}: ${MESSAGE}"

TITLE="Gemini - ${NOTIFICATION_TYPE}"
ADDRESS="127.0.0.1"
PORT="8089"

curl -X POST \
     -H "Content-Type: application/json" \
     -d "{\"message\": \"${MESSAGE:0:25}\", \"title\": \"$TITLE\", \"link\": \"\", \"detailed_message\":\"${MESSAGE}\"}" \
     "http://$ADDRESS:$PORT/interactions"

