#!/usr/bin/env python3
import os
import json
import time
import sys
import urllib.request
import hashlib
import glob

def tail_file(filename):
    """Yield lines as they are written to a file."""
    try:
        with open(filename, 'r') as f:
            # Go to the end of the file if we want to only see new messages,
            # but for a new session, we might want to start from the beginning
            # to catch the first message.
            # Actually, for agy, the file is created when the session starts.
            # We'll start from the beginning to ensure we don't miss anything.
            while True:
                line = f.readline()
                if not line:
                    # Check if file was truncated or rotated
                    if os.path.getsize(filename) < f.tell():
                        f.seek(0)
                        continue
                    time.sleep(0.1)
                    continue
                yield line
    except Exception as e:
        sys.stderr.write(f"Error reading file {filename}: {e}\n")

def send_interaction(backend_url, payload):
    """Send an interaction to the push backend."""
    try:
        req = urllib.request.Request(
            f"{backend_url}/interactions",
            data=json.dumps(payload).encode('utf-8'),
            headers={'Content-Type': 'application/json'}
        )
        with urllib.request.urlopen(req, timeout=5) as response:
            pass
    except Exception as e:
        # sys.stderr.write(f"Error sending interaction: {e}\n")
        pass

def get_latest_log_file(log_dir):
    if not os.path.exists(log_dir):
        return None
    files = glob.glob(os.path.join(log_dir, "session-*.jsonl"))
    if not files:
        return None
    return max(files, key=os.path.getmtime)

def main():
    if len(sys.argv) < 5:
        print("Usage: agy_scraper.py <log_dir> <backend_url> <fallback_session_id> <session_path>")
        sys.exit(1)

    log_dir = os.path.expanduser(sys.argv[1])
    backend_url = sys.argv[2]
    fallback_session_id = sys.argv[3]
    session_path = sys.argv[4]

    sys.stderr.write(f"Watching log directory: {log_dir}\n")
    
    current_log_file = None
    seen_messages = {} # Map id -> last hash of its data
    session_id = fallback_session_id

    while True:
        latest = get_latest_log_file(log_dir)
        if latest and latest != current_log_file:
            sys.stderr.write(f"Found new log file: {latest}\n")
            current_log_file = latest
            # Reset seen messages for new file? 
            # Usually one file per session, so yes.
            seen_messages = {}
            
            for line in tail_file(current_log_file):
                # If a newer log file appeared, switch to it
                if time.time() % 5 < 0.2: # Check every few seconds
                    new_latest = get_latest_log_file(log_dir)
                    if new_latest and new_latest != current_log_file:
                        sys.stderr.write(f"Switching to newer log file: {new_latest}\n")
                        current_log_file = new_latest
                        seen_messages = {}
                        break

                line = line.strip()
                if not line:
                    continue
                    
                try:
                    data = json.loads(line)
                    
                    if "$set" in data:
                        continue

                    msg_id = data.get("id")
                    if not msg_id:
                        continue
                    
                    # Update session_id if we find one in the log
                    # (In some formats it's top-level or in messages)
                    log_session_id = data.get("sessionId")
                    if log_session_id:
                        session_id = log_session_id

                    msg_type = data.get("type") # 'user' or 'gemini'
                    content = data.get("content", "")
                    thoughts = data.get("thoughts", [])
                    tool_calls = data.get("toolCalls", [])
                    
                    thought_text = ""
                    if thoughts:
                        thought_text = "\n\n".join([f"**{t.get('subject')}**: {t.get('description')}" for t in thoughts])

                    status = "w"
                    if msg_type == "user":
                        status = "d"
                    
                    payload = {
                        "identifier": msg_id,
                        "message": content[:100] if content else "Working...",
                        "detailed_message": content + ("\n\n### Thoughts\n" + thought_text if thought_text else ""),
                        "title": os.path.basename(session_path),
                        "agent": "antigravity",
                        "kind": "agent" if msg_type == "gemini" else "status",
                        "status": status,
                        "session_id": session_id,
                        "session_path": session_path,
                        "quiet": True
                    }
                    
                    msg_hash = hashlib.md5(line.encode()).hexdigest()
                    if seen_messages.get(msg_id) == msg_hash:
                        continue
                    
                    seen_messages[msg_id] = msg_hash
                    send_interaction(backend_url, payload)

                except Exception as e:
                    # sys.stderr.write(f"Error parsing log line: {e}\n")
                    pass
        else:
            time.sleep(1)

if __name__ == "__main__":
    main()
