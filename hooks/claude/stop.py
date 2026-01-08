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
            
        # Original: #echo $INPUT > ~/stop.json
        
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        
        message = f"{wd}: Done"
        title = "Claude - Done"
        
        payload = {
            "message": message,
            "title": title,
            "link": "",
            "detailed_message": message
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
