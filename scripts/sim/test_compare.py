import argparse
import json
from pathlib import Path
import sqlite3
import tempfile
import unittest

from compare import metrics, reconcile_after, validate_history_result


class ComparisonTests(unittest.TestCase):
    def test_reconciliation_uses_post_shutdown_rows_not_old_snapshot(self):
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory)
            label = "test-buffer16384"
            db = sqlite3.connect(output / f"{label}.db")
            db.execute("CREATE TABLE job_history(id TEXT, state TEXT, queue TEXT)")
            db.executemany("INSERT INTO job_history VALUES (?,?,?)", [
                ("sim-1", "Completed", "history-reliability"),
                ("sim-2", "Completed", "history-reliability"),
                ("sim-3", "Failed", "history-reliability"),
                ("seed-1", "Completed", "seed-only"),
            ])
            db.commit()
            db.close()
            (output / f"{label}.bullmq.json").write_text(json.dumps({
                "completedIds": ["sim-1", "sim-2"], "failedIds": ["sim-3", "sim-missing"],
            }))
            details = {
                "sqlite_live_counts": {"Completed": 1},
                "missing_ids": ["sim-2", "sim-3", "sim-missing"],
                "max_sampled_metrics": {"bullmq_workload_event_lag_seconds": 10},
                "final_metrics": {"bullmq_workload_event_lag_seconds": 40},
            }
            result = reconcile_after(argparse.Namespace(output=output), label, details)
            self.assertEqual(result["sqlite_counts_before_shutdown"], {"Completed": 1})
            self.assertEqual(result["missing_ids_before_shutdown"], ["sim-2", "sim-3", "sim-missing"])
            self.assertEqual(result["sqlite_live_counts"], {"Completed": 2, "Failed": 1})
            self.assertEqual(result["missing_ids"], ["sim-missing"])
            self.assertEqual(result["state_mismatches"], [])
            self.assertEqual(result["unexpected_ids"], [])
            self.assertEqual(result["max_sampled_metrics"]["bullmq_workload_event_lag_seconds"], 40)
            again = reconcile_after(argparse.Namespace(output=output), label, result)
            self.assertEqual(again["sqlite_counts_before_shutdown"], {"Completed": 1})

    def test_metric_parser_preserves_labels_and_missing_series(self):
        parsed = metrics(
            '# HELP example text\n'
            'bullderdash_store_writes_dropped_total 13\n'
            'bullderdash_store_write_lock_wait_seconds_sum{operation="ingest"} 1.25\n'
            'bullderdash_history_gaps_total{queue="q"} 2\n'
            'redis_operation_duration_seconds_count{operation="history_hgetall"} 12\n'
            'process_cpu_seconds_total 40\n'
        )
        self.assertEqual(parsed, {
            "bullderdash_store_writes_dropped_total": 13,
            'bullderdash_store_write_lock_wait_seconds_sum{operation="ingest"}': 1.25,
            'bullderdash_history_gaps_total{queue="q"}': 2,
            'redis_operation_duration_seconds_count{operation="history_hgetall"}': 12,
        })
        self.assertNotIn("bullderdash_store_write_retries_total", parsed)


    def test_history_result_requires_durable_tail_not_just_no_missing_jobs(self):
        result = {
            "stream_info": ["last-generated-id", "100-9"],
            "committed_cursors_after_shutdown": {"history-reliability": "100-9"},
            "rows_after_shutdown": 1000, "offered": 1000, "missing_ids": [],
            "unexpected_ids": [], "state_mismatches": [], "console_html": True, "fts_rows": 1000,
            "final_metrics": {"bullderdash_store_writes_dropped_total": 0},
        }
        validate_history_result(result, "normal")
        result["committed_cursors_after_shutdown"]["history-reliability"] = "100-8"
        with self.assertRaises(AssertionError):
            validate_history_result(result, "normal")


if __name__ == "__main__":
    unittest.main()
