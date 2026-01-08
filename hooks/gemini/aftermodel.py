#!/usr/bin/env python3
import sys
import json
import urllib.request
import os

def main():
    try:
        # Read input from stdin
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            return

        # Log input for debugging
        with open(os.path.expanduser("~/am.json"), "w") as f:
            f.write(raw_input)

        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        
        # Logic to get the last model message
        # .llm_request.messages | [.[] | select(.role == "model")] | .[-1].content
        message_content = "null"
        llm_request = data.get("llm_request", {})
        messages = llm_request.get("messages", [])
        model_messages = [m for m in messages if m.get("role") == "model"]
        if model_messages:
            message_content = model_messages[-1].get("content", "")
            
        # Finish reason
        # .llm_response.candidates[0].finishReason
        finish_reason = "null"
        llm_response = data.get("llm_response", {})
        candidates = llm_response.get("candidates", [])
        if candidates:
            finish_reason = candidates[0].get("finishReason", "null")
        short_response = llm_response.get("text") or 'null'
        # Check exit conditions
        if not finish_reason or finish_reason == "null" or short_response == "null" or message_content == "null":
            return

        notification_type = "Done"
        message = f"{wd}: {short_response}"
        detailed_message = f"{wd}: {message_content}"
        title = f"Gemini - {notification_type}"
        
        payload = {
            "message": message[:50],
            "title": title,
            "link": "",
            "detailed_message": detailed_message
        }
        
        url = "http://127.0.0.1:8089/interactions"
        req = urllib.request.Request(
            url, 
            data=json.dumps(payload).encode('utf-8'), 
            headers={'Content-Type': 'application/json'}
        )
        
        with urllib.request.urlopen(req) as response:
            pass

    except Exception as e:
        sys.stderr.write(str(e))

if __name__ == "__main__":
    main()
