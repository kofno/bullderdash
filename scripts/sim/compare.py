"""Local-only Windows stress runner for reliability.ts; all artifacts stay outside git."""
import argparse
import collections
import json
import os
from pathlib import Path
import shutil
import signal
import sqlite3
import subprocess
import threading
import time
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
BASE_ENV = {
    "REDIS_ADDR": "127.0.0.1:16389", "REDIS_USERNAME": "", "REDIS_PASSWORD": "",
    "REDIS_DB": "0", "REDIS_SENTINEL_MASTER": "", "REDIS_SENTINEL_ADDRS": "",
    "SERVER_PORT": "18089", "QUEUE_PREFIX": "bull",
    "METRICS_POLL_SECONDS": "2", "DASHBOARD_REFRESH_TIMEOUT_SECONDS": "10",
    "WORKLOAD_METRICS_ENABLED": "true", "WORKLOAD_METRICS_POLL_SECONDS": "1",
    "WORKLOAD_METRICS_BLOCK_SECONDS": "1", "WORKLOAD_METRICS_BATCH_SIZE": "100",
    "WORKLOAD_METRICS_START_ID": "$", "STORE_ENABLED": "true",
    "STORE_BATCH_SIZE": "512", "STORE_FLUSH_MILLIS": "500",
    "STORE_COMPLETED_TTL_HOURS": "1", "STORE_FAILED_TTL_HOURS": "336",
    "STORE_SWEEP_SECONDS": "10", "STORE_SWEEP_BATCH": "2000",
    "STORE_SWEEP_TIMEOUT_SECONDS": "300", "STORE_MAX_ROWS": "0",
    "STORE_READ_CONCURRENCY": "16", "STORE_SWEEP_PAUSE_MILLIS": "25",
    "STORE_TRACE_KEYS": "traceId",
}


def fetch(path, timeout=30):
    with urllib.request.urlopen("http://127.0.0.1:18089" + path, timeout=timeout) as response:
        return response.read().decode()


def metrics(text):
    return {key: float(value) for line in text.splitlines()
            if line and not line.startswith("#")
            for key, value in [line.rsplit(" ", 1)]
            if key.startswith(("bullderdash_store_", "bullmq_workload_", "bullmq_jobs_finished_"))}


def stop_app(app, log):
    try:
        if app.poll() is None:
            app.send_signal(signal.CTRL_BREAK_EVENT)
            try:
                app.wait(timeout=20)
            except subprocess.TimeoutExpired:
                app.terminate()
                app.wait(timeout=10)
                raise RuntimeError("app required forced termination")
    finally:
        log.close()


def start_app(binary, dbpath, output, env):
    log = output.open("wb")
    started = time.monotonic()
    app = subprocess.Popen([str(binary)], cwd=ROOT, env=dict(os.environ, **env, STORE_DB_PATH=str(dbpath)),
                           stdout=log, stderr=subprocess.STDOUT,
                           creationflags=subprocess.CREATE_NEW_PROCESS_GROUP)
    try:
        while True:
            if app.poll() is not None:
                raise RuntimeError(f"app exited {app.returncode}")
            try:
                fetch("/health", 1)
                return app, log, time.monotonic() - started
            except OSError:
                if time.monotonic() - started > 900:
                    raise RuntimeError("startup exceeded 15 minutes")
                time.sleep(.1)
    except BaseException:
        stop_app(app, log)
        raise


def prepare_seed(args):
    path = args.output / "seed.db"
    if path.exists():
        fixture = json.loads((args.output / "fixture.json").read_text())
        if (fixture["rows"], fixture["expired"]) != (args.rows, args.expired):
            raise RuntimeError("existing fixture metadata differs from requested row counts")
        db = sqlite3.connect(path)
        n = db.execute("SELECT count(*) FROM job_history").fetchone()[0]
        db.close()
        if n != args.rows:
            raise RuntimeError(f"partial seed: {n}/{args.rows}; preserve and inspect, do not overwrite")
        return path
    started = time.monotonic()
    app, log, _ = start_app(args.baseline, path, args.output / "seed-schema.log", BASE_ENV)
    stop_app(app, log)
    db = sqlite3.connect(path)
    db.execute("PRAGMA journal_mode=WAL")
    db.execute("CREATE TABLE stress_lock (id INTEGER PRIMARY KEY, value INTEGER)")
    db.execute("INSERT INTO stress_lock VALUES (1,0)")
    db.commit()
    now = int(time.time() * 1000)
    retained = args.rows - args.expired
    query = """INSERT INTO job_history
      (id,queue,name,state,attempts,execution_depth,trace_id,created_at,finished_at,last_error,data,opts,updated_at)
      VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?)"""
    try:
        for offset in range(0, args.rows, 2000):
            batch = []
            for i in range(offset, min(offset + 2000, args.rows)):
                old = i >= retained
                ended = now - (48 * 3_600_000 if old else 0)
                payload = json.dumps({"deviceId": f"seed-{i}", "message": "seedtelemetry",
                                      "payload": " ".join(f"sensor{(i*97+j*13)%100003}" for j in range(96))})
                batch.append((f"seed-{i}", "seed-only", "seed-expired" if old else "seed-retained",
                              "Failed" if not old and i % 10 == 0 else "Completed",
                              1, 0, f"seedtrace-{i}", ended-100, ended, "", payload, "{}", now))
            db.executemany(query, batch)
            db.commit()
            if (offset + len(batch)) % 100_000 == 0:
                print(json.dumps({"seed_rows": offset + len(batch),
                                  "elapsed_seconds": time.monotonic()-started}), flush=True)
        db.execute("INSERT INTO job_fts(job_fts,rank) VALUES ('integrity-check',1)")
        db.commit()
        db.execute("PRAGMA wal_checkpoint(TRUNCATE)")
    finally:
        db.close()
    result = {"rows": args.rows, "expired": args.expired, "retained": retained,
              "seconds": time.monotonic()-started, "bytes": path.stat().st_size}
    (args.output / "fixture.json").write_text(json.dumps(result, indent=2))
    print(json.dumps(result), flush=True)
    return path


def competing_writer(dbpath, begin, stop, result):
    db = sqlite3.connect(dbpath, timeout=.1, isolation_level=None)
    try:
        # Sweep starts 30s after app startup; burst begins ~31s after readiness.
        if stop.wait(max(0, begin + 29 - time.monotonic())):
            return
        end = begin + 119
        while time.monotonic() < end and not stop.is_set():
            requested = time.monotonic()
            try:
                db.execute("BEGIN IMMEDIATE")
                acquired = time.monotonic()
                db.execute("UPDATE stress_lock SET value=value+1 WHERE id=1")
                stop.wait(.4)
                db.execute("COMMIT")
                result.append({"at": acquired-begin, "hold_seconds": time.monotonic()-acquired,
                               "wait_seconds": acquired-requested})
            except sqlite3.OperationalError as error:
                if db.in_transaction:
                    db.rollback()
                if error.sqlite_errorcode & 0xff not in (sqlite3.SQLITE_BUSY, sqlite3.SQLITE_LOCKED):
                    result.append({"error": str(error)})
                    return
                result.append({"at": time.monotonic()-begin, "busy": True})
            stop.wait(.1)
    finally:
        db.close()


def reconcile_after(args, label, details):
    conn = sqlite3.connect(args.output / f"{label}.db")
    actual = dict(conn.execute("SELECT id,state FROM job_history WHERE queue='history-reliability'"))
    conn.close()
    manifest = json.loads((args.output / f"{label}.bullmq.json").read_text())
    expected = set(manifest["completedIds"]) | set(manifest["failedIds"])
    details.setdefault("sqlite_counts_before_shutdown", details["sqlite_live_counts"])
    details.setdefault("missing_ids_before_shutdown", details["missing_ids"])
    details["sqlite_live_counts"] = dict(collections.Counter(actual.values()))
    details["missing_ids"] = sorted(expected-set(actual))
    details["unexpected_ids"] = sorted(set(actual)-expected)
    details["state_mismatches"] = [
        id for ids, state in [(manifest["completedIds"], "Completed"), (manifest["failedIds"], "Failed")]
        for id in ids if id in actual and actual[id] != state]
    maxima = details.setdefault("max_sampled_metrics", {})
    for key, value in details.get("final_metrics", {}).items():
        maxima[key] = max(maxima.get(key, value), value)
    (args.output / f"{label}.summary.json").write_text(json.dumps(details, indent=2))
    return details


def run(args, seed, build, binary, buffer):
    label = f"{build}-buffer{buffer}"
    summary = args.output / f"{label}.summary.json"
    if summary.exists():
        details = json.loads(summary.read_text())
        if "rows_after_shutdown" not in details:
            raise RuntimeError("summary is incomplete; inspect the interrupted run before reusing")
        return reconcile_after(args, label, details)
    dbpath = args.output / f"{label}.db"
    if dbpath.exists():
        raise RuntimeError(f"incomplete run exists: {dbpath}")
    conn = sqlite3.connect(seed)
    timestamp = conn.execute(
        "SELECT finished_at FROM job_history WHERE queue='seed-only' AND id='seed-0'").fetchone()[0]
    conn.close()
    if timestamp + int(BASE_ENV["STORE_COMPLETED_TTL_HOURS"]) * 3_600_000 <= time.time() * 1000:
        raise RuntimeError("retained seed is now past TTL; build a new fixture, not a mismatched retention run")
    subprocess.run(["docker", "exec", args.container, "valkey-cli", "FLUSHDB"],
                   check=True, capture_output=True)
    copy_started = time.monotonic()
    shutil.copyfile(seed, dbpath)
    copy_seconds = time.monotonic()-copy_started
    env = dict(BASE_ENV, STORE_WRITE_BUFFER=str(buffer))
    app, log, startup = start_app(binary, dbpath, args.output / f"{label}.app.log", env)
    begin = time.monotonic()
    stop = threading.Event()
    writer_results = []
    writer = threading.Thread(target=competing_writer, args=(dbpath, begin, stop, writer_results))
    writer.start()
    maxima = collections.defaultdict(float)
    searches = []
    sim = None
    simlog = (args.output / f"{label}.sim.log").open("wb")
    snapshots = (args.output / f"{label}.metrics.jsonl").open("w")
    manifest = args.output / f"{label}.bullmq.json"
    try:
        sim_env = dict(os.environ, REDIS_URL="redis://127.0.0.1:16389", SIM_QUEUE="history-reliability",
                       SIM_OUTPUT=str(manifest), SIM_START_DELAY_SECONDS="5", SIM_STEADY_RATE="78",
                       SIM_STEADY_SECONDS="26", SIM_BURST_RATE="2000", SIM_BURST_SECONDS="60",
                       SIM_RECOVERY_SECONDS="60", SIM_DRAIN_SECONDS="180")
        sim = subprocess.Popen(["bun", "run", "reliability.ts"], cwd=ROOT / "scripts" / "sim",
                               env=sim_env, stdout=simlog, stderr=subprocess.STDOUT)
        last_search = -10
        # Continue sampling during post-sim collector catch-up and sweep completion.
        deadline = begin + 420
        final = {}
        result = None
        while time.monotonic() < deadline:
            elapsed = time.monotonic()-begin
            text = fetch("/metrics")
            final = metrics(text)
            snapshots.write(json.dumps({"seconds": elapsed, "values": final})+"\n")
            snapshots.flush()
            for key, value in final.items():
                maxima[key] = max(maxima[key], value)
            if elapsed-last_search >= 10:
                query_start = time.monotonic()
                try:
                    rows = json.loads(fetch("/v1/search?q=reliabilityneedle&limit=5", 15))
                    searches.append({"at": elapsed, "seconds": time.monotonic()-query_start, "rows": len(rows)})
                except (OSError, TimeoutError) as error:
                    searches.append({"at": elapsed, "seconds": time.monotonic()-query_start, "error": str(error)})
                last_search = elapsed
            if sim.poll() is not None:
                if sim.returncode != 0:
                    raise RuntimeError(f"sim exited {sim.returncode}")
                if result is None:
                    result = json.loads(manifest.read_text())
                observed = sum(v for k, v in final.items() if k.startswith("bullmq_jobs_finished_total"))
                persisted = final.get("bullderdash_store_writes_total", 0)
                dropped = final.get("bullderdash_store_writes_dropped_total", 0)
                if (observed == result["offered"] and persisted+dropped == observed and
                        final.get("bullderdash_store_sweep_deleted_total", 0) == args.expired and elapsed >= 160):
                    break
            time.sleep(.5)
        if result is None:
            raise RuntimeError("sim did not finish within workload deadline")
        monitor_finished = time.monotonic()-begin
        stop.set()
        writer.join(timeout=5)
        conn = sqlite3.connect(f"file:{dbpath.as_posix()}?mode=ro", uri=True, timeout=30)
        actual = dict(conn.execute("SELECT id,state FROM job_history WHERE queue='history-reliability'"))
        conn.close()
        expected = set(result["completedIds"]) | set(result["failedIds"])
        mismatches = [id for id in result["completedIds"] if id in actual and actual[id] != "Completed"]
        mismatches += [id for id in result["failedIds"] if id in actual and actual[id] != "Failed"]
        final_text = fetch("/metrics")
        (args.output / f"{label}.final.prom").write_text(final_text)
        search = json.loads(fetch("/v1/search?q=reliabilityneedle&limit=1000"))
        details = {
            "build": build, "buffer": buffer, "copy_seconds": copy_seconds, "startup_seconds": startup,
            "elapsed_seconds": time.monotonic()-begin, "settings": env, "sim_settings": {
                k: v for k, v in sim_env.items() if k.startswith("SIM_")},
            "bullmq_counts": result["counts"], "offered": result["offered"], "timeline": result["timeline"],
            "sqlite_live_counts": dict(collections.Counter(actual.values())),
            "missing_ids": sorted(expected-set(actual)), "unexpected_ids": sorted(set(actual)-expected),
            "state_mismatches": mismatches, "final_metrics": metrics(final_text),
            "max_sampled_metrics": dict(maxima), "search_samples": searches, "fts_rows": len(search),
            "console_html": "html" in fetch("/console").lower(), "competing_writer": writer_results,
            "stream_info": result["streamInfo"], "stream_length": result["eventStreamLength"],
            "monitor_finished_seconds": monitor_finished,
            "workload_deadline_reached": monitor_finished >= 420,
        }
        summary.write_text(json.dumps(details, indent=2))
        print(json.dumps({"label": label, "startup_seconds": startup, "elapsed_seconds": details["elapsed_seconds"],
                          "offered": result["offered"], "sqlite": details["sqlite_live_counts"],
                          "missing": len(details["missing_ids"]), "drops": details["final_metrics"].get(
                              "bullderdash_store_writes_dropped_total"), "swept": details["final_metrics"].get(
                              "bullderdash_store_sweep_deleted_total")}), flush=True)
    finally:
        stop.set()
        writer.join(timeout=5)
        if sim is not None and sim.poll() is None:
            sim.terminate()
            sim.wait(timeout=10)
        simlog.close()
        snapshots.close()
        stop_app(app, log)
    conn = sqlite3.connect(dbpath)
    details["seed_counts_after"] = dict(conn.execute(
        "SELECT name,count(*) FROM job_history WHERE queue='seed-only' GROUP BY name"))
    details["rows_after_shutdown"] = conn.execute(
        "SELECT count(*) FROM job_history WHERE queue='history-reliability'").fetchone()[0]
    started = time.monotonic()
    conn.execute("INSERT INTO job_fts(job_fts,rank) VALUES ('integrity-check',1)")
    conn.commit()
    details["fts_integrity_seconds"] = time.monotonic()-started
    conn.close()
    details["app_exit_code"] = app.returncode
    summary.write_text(json.dumps(details, indent=2))
    return reconcile_after(args, label, details)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--baseline", type=Path, required=True)
    parser.add_argument("--phase1", type=Path, required=True)
    parser.add_argument("--container", default="bdd-history-stress-31e5b9")
    parser.add_argument("--rows", type=int, default=3_000_000)
    parser.add_argument("--expired", type=int, default=1_000_000)
    parser.add_argument("--buffer", type=int, default=16384)
    args = parser.parse_args()
    if not 0 < args.expired < args.rows or args.buffer <= 0:
        parser.error("require 0 < expired < rows and positive buffer")
    args.output = args.output.resolve()
    args.baseline = args.baseline.resolve()
    args.phase1 = args.phase1.resolve()
    args.output.mkdir(parents=True, exist_ok=True)
    # Reject an unrelated container even though every network address is loopback.
    inspect = json.loads(subprocess.check_output(["docker", "inspect", args.container]))[0]
    bindings = inspect["NetworkSettings"]["Ports"]["6379/tcp"]
    if bindings != [{"HostIp": "127.0.0.1", "HostPort": "16389"}]:
        raise RuntimeError("expected a dedicated container published only at 127.0.0.1:16389")
    seed = prepare_seed(args)
    for build, binary in [("baseline", args.baseline), ("phase1", args.phase1)]:
        value = run(args, seed, build, binary, args.buffer)
        assert not value["unexpected_ids"] and not value["state_mismatches"]
        assert value["rows_after_shutdown"] + len(value["missing_ids"]) == value["offered"]
        assert value["console_html"] and value["fts_rows"] == 1000
    print("Paired stress comparison completed; inspect summary files for loss accounting", flush=True)


if __name__ == "__main__":
    main()
