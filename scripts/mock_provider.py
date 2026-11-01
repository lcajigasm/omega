#!/usr/bin/env python3
"""Local OpenAI-compatible server for capture recordings.

`models` only answers GET /v1/models. `sudo` answers that and, on the first
chat completion, asks omega to run `sudo -n true`. Nothing leaves the machine.
"""

import json
import sys
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

MODELS = {
    "object": "list",
    "data": [
        {"id": "omega-local-test"},
        {"id": "omega-local-small"},
    ],
}

SUDO_CALL = {
    "id": "chatcmpl-omega-capture",
    "object": "chat.completion",
    "choices": [
        {
            "index": 0,
            "finish_reason": "tool_calls",
            "message": {
                "role": "assistant",
                "content": None,
                "tool_calls": [
                    {
                        "id": "call_sudo",
                        "type": "function",
                        "function": {
                            "name": "run_command",
                            "arguments": json.dumps(
                                {
                                    "command": "sudo -n true",
                                    "reason": "Check whether admin access is available",
                                }
                            ),
                        },
                    }
                ],
            },
        }
    ],
    "usage": {"prompt_tokens": 12, "completion_tokens": 8},
}

DONE = {
    "id": "chatcmpl-omega-capture-done",
    "object": "chat.completion",
    "choices": [
        {
            "index": 0,
            "finish_reason": "stop",
            "message": {"role": "assistant", "content": "Admin check finished."},
        }
    ],
    "usage": {"prompt_tokens": 20, "completion_tokens": 4},
}


def handler(mode):
    calls = {"n": 0}

    class Handler(BaseHTTPRequestHandler):
        protocol_version = "HTTP/1.1"

        def do_GET(self):
            if self.path.rstrip("/").endswith("/models"):
                self._send(MODELS)
                return
            self.send_error(404)

        def do_POST(self):
            length = int(self.headers.get("Content-Length", "0"))
            self.rfile.read(length)
            if not self.path.rstrip("/").endswith("/chat/completions"):
                self.send_error(404)
                return
            calls["n"] += 1
            self._send(SUDO_CALL if calls["n"] == 1 else DONE)

        def _send(self, payload):
            body = json.dumps(payload).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, fmt, *args):
            sys.stderr.write("mock_provider: " + (fmt % args) + "\n")

    return Handler


def main():
    if len(sys.argv) != 3 or sys.argv[2] not in ("models", "sudo"):
        sys.stderr.write("usage: mock_provider.py PORT models|sudo\n")
        return 2
    port = int(sys.argv[1])
    server = ThreadingHTTPServer(("127.0.0.1", port), handler(sys.argv[2]))
    server.serve_forever()


if __name__ == "__main__":
    sys.exit(main())
