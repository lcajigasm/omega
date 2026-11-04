#!/usr/bin/env python3
"""Drive the omega binary until a refused read and an approval are on screen.

A local OpenAI-compatible server stands in for the model. The binary is the
real one: it refuses the private key itself and paints the TUI. Nothing calls
a paid API. Output is copied to stdout so asciinema can record it.
"""

import json
import os
import pty
import select
import signal
import struct
import sys
import termios
import threading
import time
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

ROWS = 32
COLS = 100

MODELS = {
    "object": "list",
    "data": [{"id": "omega-local-test"}, {"id": "omega-local-small"}],
}

REPORT = (
    "TOP OF THE REPORT\n\n"
    + "\n".join(f"Earlier check {i}: nothing to clean here." for i in range(1, 18))
    + "\n\nThe key file is next."
)


def completion(text, tool):
    message = {"role": "assistant", "content": text}
    finish = "stop"
    if tool is not None:
        message["content"] = text
        message["tool_calls"] = [
            {
                "id": tool["id"],
                "type": "function",
                "function": {"name": tool["name"], "arguments": json.dumps(tool["arguments"])},
            }
        ]
        finish = "tool_calls"
    return {
        "id": "chatcmpl-omega-failure",
        "object": "chat.completion",
        "choices": [{"index": 0, "finish_reason": finish, "message": message}],
        "usage": {"prompt_tokens": 20, "completion_tokens": 12},
    }


CALLS = [
    completion(
        REPORT,
        {"id": "call_read", "name": "read_file", "arguments": {"path": "~/.ssh/id_ed25519"}},
    ),
    completion(
        "The key was refused. Removing openssl is the other option.",
        {
            "id": "call_rm",
            "name": "run_command",
            "arguments": {"command": "brew uninstall openssl", "reason": "it is in the way"},
        },
    ),
    completion("Stopped. The key stayed on disk.", None),
]


def serve(port, ready):
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
            n = calls["n"]
            calls["n"] += 1
            self._send(CALLS[min(n, len(CALLS) - 1)])

        def _send(self, payload):
            body = json.dumps(payload).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, fmt, *args):
            sys.stderr.write("drive_failure: " + (fmt % args) + "\n")

    server = ThreadingHTTPServer(("127.0.0.1", port), Handler)
    ready.set()
    server.serve_forever()


def set_winsize(fd):
    packed = struct.pack("HHHH", ROWS, COLS, 0, 0)
    fcntl_ioctl = getattr(termios, "TIOCSWINSZ", 0x5414)
    import fcntl

    fcntl.ioctl(fd, fcntl_ioctl, packed)


def read_some(fd):
    try:
        return os.read(fd, 65536)
    except OSError:
        return b""


def main():
    port = int(os.environ.get("OMEGA_FAILURE_PORT", "18082"))
    ready = threading.Event()
    threading.Thread(target=serve, args=(port, ready), daemon=True).start()
    if not ready.wait(5):
        sys.stderr.write("drive_failure: mock provider did not start\n")
        return 1

    config = os.environ["XDG_CONFIG_HOME"]
    os.makedirs(os.path.join(config, "omega"), exist_ok=True)
    with open(os.path.join(config, "omega", "config.toml"), "w", encoding="utf-8") as fh:
        fh.write(
            "provider = \"ollama\"\n"
            "language = \"en\"\n"
            "approval = \"dangerous\"\n"
            "command_timeout = \"2m\"\n"
            "model = \"omega-local-test\"\n"
            "\n"
            "[providers.ollama]\n"
            "model = \"omega-local-test\"\n"
            f"base_url = \"http://127.0.0.1:{port}/v1\"\n"
        )

    binary = os.environ.get("OMEGA_BIN", "./bin/omega")
    master, slave = pty.openpty()
    set_winsize(master)
    set_winsize(slave)
    pid = os.fork()
    if pid == 0:
        os.setsid()
        os.dup2(slave, 0)
        os.dup2(slave, 1)
        os.dup2(slave, 2)
        os.close(master)
        os.close(slave)
        env = os.environ.copy()
        env["TERM"] = "xterm-256color"
        env["OMEGA_LANG"] = "en"
        os.execve(binary, [binary, "-p", "Look at the SSH key, then remove openssl"], env)
    os.close(slave)
    time.sleep(0.05)
    os.kill(pid, signal.SIGWINCH)

    sent_scroll = sent_no = False
    saw_reason = False
    full = b""
    deadline = time.time() + 90
    while time.time() < deadline:
        ready_fds, _, _ = select.select([master], [], [], 0.2)
        if master in ready_fds:
            chunk = read_some(master)
            if not chunk:
                break
            os.write(1, chunk)
            full += chunk
        text = full.decode("utf-8", "replace")
        if "refusing to read" in text:
            saw_reason = True
        if saw_reason and not sent_scroll and "Run this?" in text:
            time.sleep(0.6)
            os.write(master, b"\x1b[5~")  # PgUp
            time.sleep(0.4)
            os.write(master, b"\x1b[6~")  # PgDn
            sent_scroll = True
            time.sleep(0.5)
        elif sent_scroll and not sent_no and "Run this?" in text:
            time.sleep(0.3)
            os.write(master, b"n")
            sent_no = True
        elif sent_no and "denied" in text.lower():
            time.sleep(0.6)
            os.write(master, b"\x03")
            time.sleep(0.3)
            break
    os.close(master)
    try:
        os.waitpid(pid, 0)
    except ChildProcessError:
        pass
    if not (saw_reason and sent_scroll and sent_no):
        sys.stderr.write(
            "drive_failure: did not show the refusal and the card "
            f"(reason={saw_reason} scroll={sent_scroll} no={sent_no})\n"
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
