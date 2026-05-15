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
        with open(os.path.expanduser("~/aa.json"), "a") as f:
            f.write("\n--- " + str(os.getpid()) + " ---\n")
            f.write(raw_input)
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        prompt_response = data.get("prompt_response", "").strip()
        
        message = f"{prompt_response}"
        title = wd if wd else "Gemini"
        
        # Send only a status update to signal the end of the turn.
        # Repeating the full content here causes duplication because this hook 
        # doesn't use the stable identifier from aftermodel.py.
        payload = {
            "message": "",
            "title": title,
            "agent": "gemini",
            "kind": "status",
            "status": "r",
            "session_id": session_id,
            "session_path": cwd,
            "link": "",
            "detailed_message": "",
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
