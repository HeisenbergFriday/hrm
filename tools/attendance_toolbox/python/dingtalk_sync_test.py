from __future__ import annotations

import sys
import unittest
from datetime import datetime, timedelta, timezone
from pathlib import Path
from unittest import mock


TOOLBOX_ROOT = Path(__file__).resolve().parent
if str(TOOLBOX_ROOT) not in sys.path:
    sys.path.insert(0, str(TOOLBOX_ROOT))

import dingtalk_sync as ds  # noqa: E402


CN_TZ = timezone(timedelta(hours=8))


class DingTalkQueryWindowTests(unittest.TestCase):
    def test_query_windows_split_long_range_and_cap_future_end(self) -> None:
        fixed_now = datetime(2026, 8, 1, 12, 0, tzinfo=CN_TZ)
        query_start = datetime(2026, 1, 1, tzinfo=CN_TZ)
        query_end = datetime(2026, 11, 1, tzinfo=CN_TZ)

        with mock.patch.object(ds, "_now_cn", return_value=fixed_now):
            windows = ds._process_instance_query_windows(query_start, query_end)

        self.assertEqual(2, len(windows))
        self.assertEqual(query_start, windows[0][0])
        self.assertEqual(windows[0][1], windows[1][0])
        self.assertLessEqual(
            windows[-1][1],
            fixed_now - timedelta(seconds=ds._OAPI_QUERY_CLOCK_SKEW_SECONDS),
        )
        for window_start, window_end in windows:
            self.assertLessEqual(
                window_end - window_start,
                timedelta(days=ds._OAPI_PROCESS_INSTANCE_QUERY_MAX_DAYS),
            )

    def test_fetch_deduplicates_instances_across_query_windows(self) -> None:
        fixed_now = datetime(2026, 8, 1, 12, 0, tzinfo=CN_TZ)
        config = ds.DingTalkConfig("client-id", "client-secret", {})
        client = ds.DingTalkClient(config)
        list_calls: list[dict[str, object]] = []

        def fake_oapi_post(path: str, payload: dict[str, object]) -> dict[str, object]:
            if path == "topapi/processinstance/listids":
                list_calls.append(payload)
                instance_ids = ["pi-1", "pi-shared"] if len(list_calls) == 1 else ["pi-shared", "pi-2"]
                return {"errcode": 0, "result": {"list": instance_ids, "next_cursor": 0}}
            instance_id = str(payload["process_instance_id"])
            return {"errcode": 0, "process_instance": {"process_instance_id": instance_id}}

        with mock.patch.object(ds, "_now_cn", return_value=fixed_now):
            with mock.patch.object(client, "oapi_post", side_effect=fake_oapi_post):
                result = client.fetch_process_instances_by_time(
                    "PROC_TRANSFER",
                    datetime(2026, 1, 1, tzinfo=CN_TZ),
                    datetime(2026, 11, 1, tzinfo=CN_TZ),
                )

        self.assertEqual(2, len(list_calls))
        self.assertEqual(["pi-1", "pi-shared", "pi-2"], result.instance_ids)
        self.assertEqual(4, result.raw_instance_id_count)
        self.assertEqual(1, result.duplicate_instance_id_count)
        self.assertEqual(3, result.detail_count)
        for call in list_calls:
            self.assertLessEqual(
                int(call["end_time"]) - int(call["start_time"]),
                ds._OAPI_PROCESS_INSTANCE_QUERY_MAX_DAYS * 24 * 60 * 60 * 1000,
            )
            self.assertLessEqual(
                int(call["end_time"]),
                int((fixed_now - timedelta(seconds=ds._OAPI_QUERY_CLOCK_SKEW_SECONDS)).timestamp() * 1000),
            )

    def test_fetch_applies_instance_limit_across_query_windows(self) -> None:
        fixed_now = datetime(2026, 8, 1, 12, 0, tzinfo=CN_TZ)
        client = ds.DingTalkClient(ds.DingTalkConfig("client-id", "client-secret", {}))
        list_call_count = 0

        def fake_oapi_post(path: str, payload: dict[str, object]) -> dict[str, object]:
            nonlocal list_call_count
            if path == "topapi/processinstance/listids":
                list_call_count += 1
                instance_ids = ["pi-1"] if list_call_count == 1 else ["pi-2", "pi-3"]
                return {"errcode": 0, "result": {"list": instance_ids, "next_cursor": 0}}
            instance_id = str(payload["process_instance_id"])
            return {"errcode": 0, "process_instance": {"process_instance_id": instance_id}}

        with mock.patch.object(ds, "_now_cn", return_value=fixed_now):
            with mock.patch.object(client, "oapi_post", side_effect=fake_oapi_post):
                result = client.fetch_process_instances_by_time(
                    "PROC_TRANSFER",
                    datetime(2026, 1, 1, tzinfo=CN_TZ),
                    datetime(2026, 11, 1, tzinfo=CN_TZ),
                    max_instances=2,
                )

        self.assertEqual(2, list_call_count)
        self.assertEqual(["pi-1", "pi-2"], result.instance_ids)
        self.assertTrue(result.truncated)
        self.assertEqual(2, result.detail_count)


if __name__ == "__main__":
    unittest.main()
