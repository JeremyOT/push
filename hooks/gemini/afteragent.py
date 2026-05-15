#!/usr/bin/env python3
import sys
import json
import urllib.request
import os

def merge_strings(existing, update):
    if not update:
        return existing
    if not existing:
        return update
    
    # Find maximal overlap
    max_overlap = 0
    limit = min(len(existing), len(update))
    for k in range(1, limit + 1):
        if existing.endswith(update[:k]):
            max_overlap = k
    
    return existing + update[max_overlap:]

def deduplicate_response(s):
    if not s or len(s) < 20:
        return s
        
    best_merged = s
    min_len = len(s)
    
    # Try different split points to see if the string was formed by 
    # appending an overlapping chunk.
    # We look for a split point where the second part overlaps significantly 
    # with the first part.
    for i in range(len(s) // 4, len(s)):
        existing = s[:i]
        update = s[i:]
        
        # Check overlap
        limit = min(len(existing), len(update))
        max_overlap = 0
        for k in range(1, limit + 1):
            if existing.endswith(update[:k]):
                max_overlap = k
        
        if max_overlap > 15: # Significant overlap threshold
            merged = existing + update[max_overlap:]
            if len(merged) < min_len:
                min_len = len(merged)
                best_merged = merged
                
    return best_merged

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
        
        # Deduplicate response
        message = deduplicate_response(prompt_response)
        if not message:
            message = "Turn complete"
        
        title = wd if wd else "Gemini"
        
        payload = {
            "identifier": "", # New message
            "message": message[:50],
            "title": title,
            "agent": "gemini",
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
