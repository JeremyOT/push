#!/usr/bin/env python3
import sys
import json
import urllib.request
import os

def main():
    try:
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            return
            
        with open(os.path.expanduser("~/note.json"), "w") as f:
            f.write(raw_input)
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        
        msg_text = data.get("message", "")
        notification_type = data.get("notification_type", "")
        details = data.get("details", {})
        
        message = f"{wd}: {msg_text}"
        title = f"Gemini - {notification_type}"
        
        payload = {
            "message": msg_text,
            "title": title,
            "agent": "gemini",
            "kind": "status", # Default to status
            "session_id": session_id,
            "session_path": cwd,
            "link": "",
            "detailed_message": json.dumps(details) if details else msg_text,
            "quiet": False
        }
        
        # If it's a tool permission, we want the UI to treat it as an approval
        if notification_type == "ToolPermission":
            payload["kind"] = "approval"
            payload["status"] = "awaiting" # Special status for approvals
            # The frontend will map this based on the title containing 'ToolPermission'
            
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
