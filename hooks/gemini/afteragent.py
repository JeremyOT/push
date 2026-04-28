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
            
        # Log input for debugging (matching original script behavior)
        #with open(os.path.expanduser("~/aa.json"), "w") as f:
        #   f.write(raw_input)
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        prompt_response = data.get("prompt_response", "")
        
        message = f"{prompt_response}"
        title = wd if wd else "Gemini"
        
        payload = {
            "message": message[:50],
            "title": title,
            "agent": "gemini",
            "status": "d",
            "session_id": session_id,
            "link": "",
            "detailed_message": message,
            "quiet": False #True
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
        # Fail silently or log to stderr if needed
        sys.stderr.write(str(e))

if __name__ == "__main__":
    main()
