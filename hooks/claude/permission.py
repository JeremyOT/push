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
            
        # Log input (commented out in original, but we can implement it or skip. Original had: #echo "${INPUT}" > ~/permission.json)
        # We'll skip logging to be cleaner, or follow the commented out intention if needed. 
        # But wait, later it does: cat "${WD}" > ~/pmwd.json and cat "${MESSAGE}" > ~/pm.json
        # I will implement the logic that constructs the message.
        
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        
        tool_name = data.get("tool_name", "")
        # tool_input = data.get("tool_input", "") # Used in commented out detailed message
        
        message = f"{wd}: Permission Request {tool_name}"
        detailed_message = f"{wd}: Permission Request {tool_name}"
        
        # Debug files from original script
        # cat "${WD}" > ~/pmwd.json -> It was writing the variable content, not file at path WD. 
        # variable WD is the basename of current directory.
        with open(os.path.expanduser("~/pmwd.json"), "w") as f:
            f.write(wd)
        
        with open(os.path.expanduser("~/pm.json"), "w") as f:
            f.write(message)
            
        title = "Claude - Permission Request"
        
        payload = {
            "message": message,
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
