#!/usr/bin/env python3
import sys
import json
import urllib.request
import os

def deduplicate_response(s):
    if not s:
        return s
        
    # Create normalized version (no whitespace) with index mapping
    normalized = []
    for i, char in enumerate(s):
        if not char.isspace():
            normalized.append((char, i))
            
    if len(normalized) < 40:
        return s
        
    dense_str = "".join(c[0] for c in normalized)
    num_chars = len(dense_str)
    
    # Find the longest repeating suffix in the dense string.
    # We try lengths from half the total non-whitespace characters down to 30.
    for length in range(num_chars // 2, 29, -1):
        suffix = dense_str[-length:]
        # Search for this suffix earlier in the dense string.
        first_idx = dense_str.find(suffix, 0, num_chars - length)
        if first_idx != -1:
            # Found a match in the dense string!
            # The cut point is the original index of the last character 
            # of the first occurrence.
            orig_idx = normalized[first_idx + length - 1][1]
            return s[:orig_idx + 1].strip()
            
    return s

def main():
    try:
        # Read input from stdin
        raw_input = sys.stdin.read()
        if not raw_input.strip():
            return
            
        # Log input for debugging
        log_path = os.path.expanduser("~/aa.json")
        try:
            with open(log_path, "a") as f:
                f.write("\n--- " + str(os.getpid()) + " ---\n")
                f.write(raw_input)
        except Exception:
            pass
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        prompt_response = data.get("prompt_response", "").strip()
        
        # Deduplicate response
        message = deduplicate_response(prompt_response)
        if not message:
            message = "Turn complete"
        
        title = wd if wd else "Antigravity"
        
        payload = {
            "identifier": "", # New message
            "message": message[:50],
            "title": title,
            "agent": "antigravity",
            "kind": "agent",
            "status": "r",
            "session_id": session_id,
            "session_path": cwd,
            "link": "",
            "detailed_message": message,
            "quiet": False
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
