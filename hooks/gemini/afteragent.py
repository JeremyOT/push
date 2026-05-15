#!/usr/bin/env python3
import sys
import json
import urllib.request
import os

import re

def deduplicate_response(s):
    if not s:
        return s
        
    # Find all non-whitespace tokens and their spans
    tokens = list(re.finditer(r'\S+', s))
    if len(tokens) < 40:
        return s
        
    token_texts = [m.group(0) for m in tokens]
    num_tokens = len(token_texts)
    
    # Look for the longest repeating suffix of tokens.
    # We try lengths from half the total tokens down to 20.
    for length in range(num_tokens // 2, 19, -1):
        suffix = token_texts[-length:]
        # Search for this token sequence earlier in the message
        for i in range(num_tokens - 2 * length + 1):
            if token_texts[i:i+length] == suffix:
                # Found a significant repeating sequence!
                # The cut point is the end of the first occurrence.
                cut_point = tokens[i + length - 1].end()
                return s[:cut_point].strip()
                
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
