# -*- coding: utf-8 -*-
"""Regression tests for part-time attendance priority rules."""
from __future__ import annotations

import sys
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parent
PARTTIME_ROOT = ROOT / "parttime"
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))
if str(PARTTIME_ROOT) not in sys.path:
    sys.path.insert(0, str(PARTTIME_ROOT))

import calc_parttime_summary as parttime  # noqa: E402


class OffsitePersonalLeavePriorityTests(unittest.TestCase):
    def test_business_trip_with_personal_leave_is_not_counted(self) -> None:
        text = "出差07-23 09:00到07-23 18:30 1.0天,事假07-23 09:00到07-23 18:30 8小时\n(-)"

        result = parttime._parse_daily_text_value(
            text,
            count_outing_as_present=True,
        )

        self.assertIsNone(result)

    def test_outing_with_personal_leave_is_not_counted(self) -> None:
        text = "外出07-23 09:00到07-23 18:30,事假07-23 09:00到07-23 18:30 8小时\n(-)"

        result = parttime._parse_daily_text_value(
            text,
            count_outing_as_present=True,
        )

        self.assertIsNone(result)

    def test_offsite_without_personal_leave_still_counts_as_present(self) -> None:
        for text in ("外出07-23 09:00到07-23 18:30\n(-)", "出差07-23 09:00到07-23 18:30 1.0天\n(-)"):
            with self.subTest(text=text):
                result = parttime._parse_daily_text_value(
                    text,
                    count_outing_as_present=True,
                )

                self.assertIsNotNone(result)
                self.assertEqual(parttime._entry_value(result), 1.0)


if __name__ == "__main__":
    unittest.main()
