#!/usr/bin/env python3
import sys
import json
import urllib.request
import os
import hashlib
import time

def main():
    try:
        # Read input from stdin
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            return

        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        
        llm_request = data.get("llm_request")
        llm_response = data.get("llm_response", {})
        short_response = llm_response.get("text") or ''

        # Generate stable identifier from request
        identifier = ""
        if llm_request:
            # We want to hash the request up until the last user message.
            # This ensures that multiple model updates for the same user turn
            # result in the same identifier.
            filtered_request = llm_request.copy()
            contents = filtered_request.get("contents", [])
            last_user_idx = -1
            for i, msg in enumerate(contents):
                if msg.get("role") == "user":
                    last_user_idx = i
            
            if last_user_idx != -1:
                filtered_request["contents"] = contents[:last_user_idx + 1]
            
            req_str = json.dumps(filtered_request, sort_keys=True)
            identifier = f"antigravity-req-{hashlib.sha256(req_str.encode()).hexdigest()[:16]}"

        # Logic to get the current model message
        message_content = short_response
        candidates = llm_response.get("candidates", [])
        if candidates:
            # Try to get the full content from the candidate if text is short
            candidate_content = candidates[0].get("content", {})
            parts = candidate_content.get("parts", [])
            if parts:
                message_content = "".join([p if isinstance(p, str) else p.get("text", "") for p in parts])
            
        # Finish reason
        finish_reason = "null"
        if candidates:
            finish_reason = candidates[0].get("finishReason", "null")

        status = "w"
        # Handle both string and integer finish reasons
        # STOP=1, MAX_TOKENS=2, SAFETY=3, RECITATION=4, OTHER=5 in some API versions
        done_reasons = ["STOP", "MAX_TOKENS", "SAFETY", "RECITATION", "OTHER", 1, 2, 3, 4, 5]
        if finish_reason in done_reasons:
             status = "d"
        else:
             status = "w"

        message = short_response
        detailed_message = f"{message_content}"
        title = wd if wd else "Antigravity"

        # Don't send empty messages unless it's a status update
        if not message_content.strip():
            if status == "d":
                # Mark as done but don't replace content with empty string
                payload = {
                    "identifier": identifier,
                    "replace": False, # Important: don't clear existing text
                    "message": "",
                    "detailed_message": "",
                    "status": "d",
                    "session_id": session_id,
                    "session_path": cwd,
                    "quiet": True
                }
            else:
                return
        else:
            payload = {
                "identifier": identifier,
                "replace": False,
                "message": message[:50],
                "title": title,
                "agent": "antigravity",
                "kind": "agent",
                "status": status,
                "session_id": session_id,
                "session_path": cwd,
                "link": "",
                "detailed_message": detailed_message,
                "quiet": True
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
