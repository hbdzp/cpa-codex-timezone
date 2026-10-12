#!/usr/bin/env python3
"""Minimal Codex Responses upstream that records every request body.

Each POST is appended as one JSON line to the file given by --log, and a
completed Responses SSE stream is returned so CPA treats the call as a success.
"""
import argparse
import json
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def sse(model: str) -> bytes:
    resp = {
        "id": "resp_mock",
        "object": "response",
        "created_at": int(time.time()),
        "status": "completed",
        "model": model,
        "output": [{
            "type": "message",
            "id": "msg_mock",
            "role": "assistant",
            "status": "completed",
            "content": [{"type": "output_text", "text": "ok", "annotations": []}],
        }],
        "usage": {"input_tokens": 1, "output_tokens": 1, "total_tokens": 2},
    }
    events = [
        {"type": "response.created", "response": {**resp, "status": "in_progress", "output": []}},
        {"type": "response.output_text.delta", "item_id": "msg_mock", "output_index": 0, "content_index": 0, "delta": "ok"},
        {"type": "response.completed", "response": resp},
    ]
    return "".join(f"event: {e['type']}\ndata: {json.dumps(e)}\n\n" for e in events).encode()


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", type=int, default=47391)
    ap.add_argument("--log", required=True)
    args = ap.parse_args()

    class Handler(BaseHTTPRequestHandler):
        def do_POST(self):
            body = self.rfile.read(int(self.headers.get("Content-Length") or 0))
            with open(args.log, "a", encoding="utf-8") as f:
                f.write(json.dumps({"path": self.path, "body": json.loads(body or b"{}")}, ensure_ascii=False) + "\n")
            model = (json.loads(body or b"{}").get("model") or "gpt-5.5")
            out = sse(model)
            self.send_response(200)
            self.send_header("Content-Type", "text/event-stream")
            self.send_header("Content-Length", str(len(out)))
            self.end_headers()
            self.wfile.write(out)

        def log_message(self, *_):
            pass

    ThreadingHTTPServer(("127.0.0.1", args.port), Handler).serve_forever()


if __name__ == "__main__":
    main()
