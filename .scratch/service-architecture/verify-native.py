"""Isolated native lifecycle and analytics measurements; never uses real sources."""

import json
import os
from pathlib import Path
import shutil
import sqlite3
import statistics
import subprocess
import tempfile
import time
import urllib.request


REPO = Path(__file__).resolve().parents[2]
BINARY = REPO / "packages/cli/bin/tokeninsights"
root = Path(tempfile.mkdtemp(prefix="ti-native-"))
db = root / "usage.sqlite"
env = dict(os.environ)
env.update(HOME=str(root), XDG_CONFIG_HOME=str(root / "config"),
           XDG_STATE_HOME=str(root / "state"), XDG_RUNTIME_DIR=str(root / "run"),
           XDG_DATA_HOME=str(root / "data"), CODEX_HOME=str(root / "codex"),
           CLAUDE_CONFIG_DIR=str(root / "claude"), TOKENINSIGHTS_DB_PATH=str(db), PATH="")
(root / "run").mkdir(mode=0o700)
sessions = root / ".pi/agent/sessions"
sessions.mkdir(parents=True)
(sessions / "session.jsonl").write_text(
    '{"type":"session","version":1,"id":"native","timestamp":"2026-09-01T00:00:00Z"}\n'
    '{"type":"message","id":"m1","timestamp":"2026-09-01T00:00:01Z",'
    '"message":{"role":"assistant","provider":"openai","model":"test",'
    '"usage":{"input":100,"output":20,"totalTokens":120}}}\n')


def cli(*args, expected=0):
    result = subprocess.run([str(BINARY), *args], env=env, capture_output=True,
                            text=True, timeout=60)
    assert result.returncode == expected, (args, result.returncode, result.stdout, result.stderr)
    return result.stdout


def status():
    return json.loads(cli("service", "status", "--json"))


def get(path):
    with urllib.request.urlopen(url + path, timeout=60) as response:
        return response.read()


def count_jobs():
    with sqlite3.connect(db) as connection:
        return connection.execute("SELECT COUNT(*) FROM sync_jobs").fetchone()[0]


def measure(function, repetitions=3):
    samples = []
    for _ in range(repetitions):
        started = time.perf_counter()
        function()
        samples.append(round((time.perf_counter() - started) * 1000, 2))
    return {"median_ms": statistics.median(samples), "samples_ms": samples}


try:
    assert json.loads(cli("service", "status", "--json", expected=3)) == {"running": False}
    assert not db.exists()
    for command in ("view", "sync", "normalize", "reset-all", "reset-canonical", "refresh", "serve"):
        cli(command, "--help")
    assert not db.exists()
    cli("service", "start", "--port", "0")
    first = status()
    url = first["service"]["url"]
    assert count_jobs() == 0
    assert b"<html" in get("/")
    get("/api/v1/usage?period=all")
    cli()
    assert status()["service"]["instanceId"] == first["service"]["instanceId"]
    assert count_jobs() == 0
    cli("refresh", "--wait")
    with sqlite3.connect(db) as connection:
        assert connection.execute("SELECT COUNT(*) FROM canonical_token_usage").fetchone()[0] == 1
    jobs = count_jobs()
    timings = {"warm_start": measure(lambda: cli()), "status": measure(status)}
    cli("service", "restart", "--host", "0.0.0.0", "--port", "0")
    current = status()
    assert current["service"]["config"]["host"] == "0.0.0.0"
    assert current["service"]["instanceId"] != first["service"]["instanceId"]
    assert count_jobs() == jobs
    url = current["service"]["url"]
    get("/api/v1/usage?period=all")
    pid = current["service"]["pid"]
    descriptor_before = len(list(Path(f"/proc/{pid}/fd").iterdir()))
    for _ in range(40):
        get("/api/v1/sync")
    descriptor_after = len(list(Path(f"/proc/{pid}/fd").iterdir()))
    assert count_jobs() == jobs
    assert descriptor_after <= descriptor_before + 2
    cli("service", "stop")

    # Existing schema, synthetic facts only. No DDL or schema contract changes.
    scale = []
    previous = 0
    for count in (10_000, 100_000):
        with sqlite3.connect(db) as connection:
            connection.executemany(
                "INSERT INTO canonical_sessions (id,semantic_key,harness,session_id,first_seen_at_ms,last_seen_at_ms) VALUES (?,?, 'pi',?,1770000000000,1770000000000)",
                ((i + 2, f"scale-{i}", f"scale-{i}") for i in range(previous, count)))
            connection.executemany(
                "INSERT INTO raw_token_usage (id,raw_fact_key,harness,source_id,source_kind,collector,parser,observed_at_ms,session_id,usage_scope,quality) VALUES (?,?,'pi','scale','test','test','test',1770000000000,?,'message','exact')",
                ((i + 2, f"scale-{i}", f"scale-{i}") for i in range(previous, count)))
            connection.executemany(
                "INSERT INTO canonical_token_usage (semantic_key,recorded_at_ms,harness,session_id,provider,model,usage_scope,quality,is_countable,input_tokens,output_tokens,total_tokens,primary_raw_fact_id) VALUES (?,1770000000000,'pi',?,'openai','test','message','exact',1,100,20,120,?)",
                ((f"scale-{i}", i + 2, i + 2) for i in range(previous, count)))
        cli("service", "start")
        current = status()
        url = current["service"]["url"]
        result = {"sessions": count + 1,
                  "usage": measure(lambda: get("/api/v1/usage?period=all&tab=sessions")),
                  "facets": measure(lambda: get("/api/v1/usage/facets?period=all")),
                  "status": measure(status), "database_bytes": db.stat().st_size}
        result["resident_memory"] = next(line for line in Path(f'/proc/{current["service"]["pid"]}/status').read_text().splitlines() if line.startswith("VmRSS:"))
        scale.append(result)
        assert count_jobs() == jobs
        cli("service", "stop")
        previous = count
    print(json.dumps({"native_path": "empty; no Node/npm/pnpm or shell helpers", "timings": timings,
                      "idle_descriptors_before": descriptor_before, "idle_descriptors_after": descriptor_after,
                      "scale": scale}, indent=2), flush=True)
    cli("service", "status", expected=3)
finally:
    cli("service", "stop")
    shutil.rmtree(root)
