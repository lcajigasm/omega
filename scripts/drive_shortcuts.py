#!/usr/bin/env python3
"""Drive the omega binary through ?, Esc and a denial.

The built-in demo provider supplies the session, so nothing calls a paid API
and no real maintenance command runs. Output of the terminal is copied to
stdout for asciinema.
"""

import os
import pty
import select
import signal
import struct
import sys
import termios
import time

ROWS = 32
COLS = 100


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
        os.execve(binary, [binary, "--demo"], env)
    os.close(slave)
    time.sleep(0.05)
    os.kill(pid, signal.SIGWINCH)

    sent_help = sent_esc = sent_no = False
    mark = 0
    full = b""
    deadline = time.time() + 120
    while time.time() < deadline:
        ready, _, _ = select.select([master], [], [], 0.2)
        if master in ready:
            chunk = read_some(master)
            if not chunk:
                break
            os.write(1, chunk)
            full += chunk
        text = full.decode("utf-8", "replace")
        fresh = text[mark:]
        if not sent_help and "Run this?" in text:
            time.sleep(0.5)
            os.write(master, b"?")
            sent_help = True
            mark = len(text)
        elif sent_help and not sent_esc and "Keyboard shortcuts" in fresh:
            time.sleep(0.9)
            os.write(master, b"\x1b")
            sent_esc = True
            mark = len(full.decode("utf-8", "replace"))
        elif sent_esc and not sent_no and "Run this?" in fresh:
            time.sleep(0.4)
            os.write(master, b"n")
            sent_no = True
            mark = len(full.decode("utf-8", "replace"))
        elif sent_no and "denied" in fresh.lower():
            time.sleep(0.8)
            os.write(master, b"\x03")
            time.sleep(0.3)
            break
    os.close(master)
    try:
        os.waitpid(pid, 0)
    except ChildProcessError:
        pass
    if not (sent_help and sent_esc and sent_no):
        sys.stderr.write(
            "drive_shortcuts: did not reach ?, Esc and a denial "
            f"(help={sent_help} esc={sent_esc} no={sent_no})\n"
        )
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
