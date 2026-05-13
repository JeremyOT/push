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
            
        data = json.loads(raw_input)
    except Exception:
        return

    try:
        hook_event_name = data.get("hook_event_name")
        tool_name = data.get("tool_name")
        
        # Only handle beforetool for ask_user
        if hook_event_name != "BeforeTool" or tool_name != "ask_user":
            return

        cwd = data.get("cwd", "")
        wd = os.path.basename(cwd) if cwd else ""
        session_id = data.get("session_id", "")
        tool_input = data.get("tool_input", {})
        
        # Format a summary message
        questions = tool_input.get("questions", [])
        if not questions:
            return
            
        summary = questions[0].get("question", "Question asked")
        if len(questions) > 1:
            summary += f" (+{len(questions)-1} more)"

        payload = {
            "message": summary,
            "title": f"{wd} - Question" if wd else "Gemini - Question",
            "agent": "gemini",
            "status": "w",
            "session_id": session_id,
            "session_path": cwd,
            "detailed_message": json.dumps(tool_input),
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
        sys.stderr.write(str(e))

if __name__ == "__main__":
    main()
