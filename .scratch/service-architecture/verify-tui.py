"""Verify the actual local TUI can quit while shared refresh waits for a writer."""

import fcntl
import json
import os
from pathlib import Path
import select
import shutil
import sqlite3
import struct
import subprocess
import tempfile
import termios
import time


binary = Path(__file__).resolve().parents[2] / "packages/cli/bin/tokeninsights"
root = Path(tempfile.mkdtemp(prefix="ti-tui-"))
db = root / "usage.sqlite"
env = dict(os.environ)
env.update(HOME=str(root), XDG_CONFIG_HOME=str(root / "config"),
           XDG_STATE_HOME=str(root / "state"), XDG_RUNTIME_DIR=str(root / "run"),
           XDG_DATA_HOME=str(root / "data"), CODEX_HOME=str(root / "codex"),
           CLAUDE_CONFIG_DIR=str(root / "claude"), TOKENINSIGHTS_DB_PATH=str(db),
           TERM="xterm-256color", PATH="")
(root / "run").mkdir(mode=0o700)
sources = root / ".pi/agent/sessions"
sources.mkdir(parents=True)
(sources / "s.jsonl").write_text(
    '{"type":"session","version":1,"id":"tui","timestamp":"2026-09-01T00:00:00Z"}\n'
    '{"type":"message","id":"m1","timestamp":"2026-09-01T00:00:01Z",'
    '"message":{"role":"assistant","provider":"openai","model":"test",'
    '"usage":{"input":100,"output":20,"totalTokens":120}}}\n')


def cli(*args):
    return subprocess.check_output([str(binary), *args], env=env, text=True, timeout=30)


def status():
    return json.loads(cli("service", "status", "--json"))


def wait_for(predicate):
    deadline = time.monotonic() + 10
    while time.monotonic() < deadline:
        if predicate():
            return
        time.sleep(0.05)
    raise AssertionError("timed out")


def open_tui(*args):
    master, slave = os.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack("HHHH", 35, 120, 0, 0))
    process = subprocess.Popen([str(binary), "view", "--all-time", *args], env=env,
                               stdin=slave, stdout=slave, stderr=slave, start_new_session=True,
                               preexec_fn=lambda: fcntl.ioctl(0, termios.TIOCSCTTY, 0))
    os.close(slave)
    return process, master


def quit_tui(process, master):
    os.write(master, b"q")
    result = process.wait(timeout=10)
    if result != 0:
        output = bytearray()
        while select.select([master], [], [], 0.1)[0]:
            try:
                output.extend(os.read(master, 65536))
            except OSError:
                break
        raise AssertionError((result, output.decode(errors="replace")[-2000:]))
    os.close(master)


process = None
try:
    cli("service", "start", "--port", "0")
    pid = status()["service"]["pid"]
    with open(str(db) + ".lock", "r+") as lock:
        fcntl.flock(lock, fcntl.LOCK_EX)
        process, master = open_tui()
        wait_for(lambda: status()["status"].get("active", {}).get("kind") == "refresh")
        quit_tui(process, master)
        assert status()["status"]["active"]["kind"] == "refresh"
        fcntl.flock(lock, fcntl.LOCK_UN)
    wait_for(lambda: not status()["status"]["running"])
    with sqlite3.connect(db) as connection:
        assert connection.execute("SELECT COUNT(*) FROM canonical_token_usage").fetchone()[0] == 1
        assert connection.execute("SELECT COUNT(*) FROM sync_jobs").fetchone()[0] == 1
    process, master = open_tui("--no-sync")
    output = bytearray()

    def renders_saved_usage():
        if select.select([master], [], [], 0.1)[0]:
            output.extend(os.read(master, 65536))
        return b"120" in output

    wait_for(renders_saved_usage)
    quit_tui(process, master)
    assert status()["service"]["pid"] == pid
    with sqlite3.connect(db) as connection:
        assert connection.execute("SELECT COUNT(*) FROM sync_jobs").fetchone()[0] == 1
    print("PASS: local TUI quit preserves accepted refresh; saved read-only reopen creates no jobs; daemon PID unchanged.")
finally:
    if process is not None and process.poll() is None:
        process.terminate()
        process.wait(timeout=10)
    cli("service", "stop")
    shutil.rmtree(root)
