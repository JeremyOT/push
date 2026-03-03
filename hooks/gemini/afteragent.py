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
        with open(os.path.expanduser("~/aa.json"), "w") as f:
            f.write(raw_input)
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        prompt_response = data.get("prompt_response", "")
        
        message = f"{wd}: {prompt_response}"
        title = "Gemini - Done"
        
        payload = {
            "message": message[:50],
            "title": title,
            "link": "",
            "detailed_message": message,
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
        # Fail silently or log to stderr if needed
        sys.stderr.write(str(e))

if __name__ == "__main__":
    main()
