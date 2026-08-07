from __future__ import annotations

import sys
import tempfile
import unittest
from datetime import date, datetime
from pathlib import Path

from openpyxl import load_workbook

BASE = Path(__file__).resolve().parent
LEAVE_DIR = str(BASE / "leave")
if LEAVE_DIR not in sys.path:
    sys.path.insert(0, LEAVE_DIR)

import calc_leave  # noqa: E402


class WarehouseScheduleLeaveTest(unittest.TestCase):
    def setUp(self):
        self.schedule_ctx = {
            "year": 2026,
            "month": 7,
            "month_start": date(2026, 7, 1),
            "month_end": date(2026, 7, 31),
            "next_month_start": date(2026, 8, 1),
            "next_month_label": "八月考勤",
            "main_working_days": {date(2026, 7, 27)},
            "chengdu_working_days": {date(2026, 7, 27)},
            "main_expected_attendance_days": {date(2026, 7, 27)},
            "chengdu_expected_attendance_days": {date(2026, 7, 27)},
        }
        self.leave_row = (
            "MT0584",
            "苏炳滔",
            "运营管理中心",
            "供应链管理部",
            "仓库小组",
            "年假",
            datetime(2026, 7, 26, 9, 0),
            datetime(2026, 7, 26, 18, 30),
            8,
        )

    def test_weekend_is_zero_for_regular_employee_but_eight_hours_for_warehouse(self):
        regular = calc_leave.calc_final_fields(self.leave_row, self.schedule_ctx)
        warehouse = calc_leave.calc_final_fields(
            self.leave_row,
            self.schedule_ctx,
            is_all_month_scheduled=True,
        )

        self.assertEqual(regular[:2], (0, 0))
        self.assertEqual(warehouse[:2], (8.0, 1.0))

    def test_process_applies_warehouse_name_list(self):
        header = tuple(f"列{index}" for index in range(16))
        source_row = self.leave_row + (
            None,
            None,
            "APPROVAL-1",
            "完成",
            "同意",
            False,
            2,
        )

        with tempfile.TemporaryDirectory() as temp_dir:
            output = Path(temp_dir) / "请假明细表.xlsx"
            calc_leave.process(
                [header, source_row],
                str(output),
                self.schedule_ctx,
                warehouse_schedule_names=("苏炳滔",),
            )
            workbook = load_workbook(output, data_only=True)
            row = [cell.value for cell in workbook["请假明细"][2]]
            workbook.close()

        self.assertEqual(row[9], 8)
        self.assertEqual(row[10], 1)

    def test_cross_month_warehouse_leave_counts_every_target_month_day(self):
        row = list(self.leave_row)
        row[6] = datetime(2026, 7, 30, 9, 0)
        row[7] = datetime(2026, 8, 2, 18, 30)
        row[8] = "4天"

        final_hours, final_days, _ = calc_leave.calc_final_fields(
            tuple(row),
            self.schedule_ctx,
            is_all_month_scheduled=True,
        )

        self.assertEqual(final_hours, 16)
        self.assertEqual(final_days, 2)


if __name__ == "__main__":
    unittest.main()
