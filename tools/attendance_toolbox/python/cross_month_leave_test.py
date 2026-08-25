from __future__ import annotations

import sys
import tempfile
import unittest
from datetime import date, datetime, timedelta
from pathlib import Path

import openpyxl


BASE = Path(__file__).resolve().parent
LEAVE_DIR = str(BASE / "leave")
if LEAVE_DIR not in sys.path:
    sys.path.insert(0, LEAVE_DIR)
FINAL_DIR = str(BASE / "finally")
if FINAL_DIR not in sys.path:
    sys.path.insert(0, FINAL_DIR)

import calc_leave  # noqa: E402
import calc_finally  # noqa: E402


def schedule_context(year: int, month: int, working_days: set[date]) -> dict:
    month_start = date(year, month, 1)
    next_month_start = calc_leave._next_month_start(year, month)
    return {
        "year": year,
        "month": month,
        "month_start": month_start,
        "month_end": next_month_start - timedelta(days=1),
        "next_month_start": next_month_start,
        "next_month_label": f"{next_month_start.month}月考勤",
        "main_working_days": set(working_days),
        "chengdu_working_days": set(working_days),
        "main_expected_attendance_days": set(working_days),
        "chengdu_expected_attendance_days": set(working_days),
    }


class CrossMonthLeaveAllocationTests(unittest.TestCase):
    def test_user_case_allocates_16_hours_to_july_and_4_to_august(self):
        row = (
            "MT0040",
            "测试员工",
            "智慧零售",
            "网点拓展部",
            "区域",
            "事假",
            datetime(2026, 7, 30, 8, 0),
            datetime(2026, 8, 1, 12, 7),
            20,
        )
        july = schedule_context(
            2026,
            7,
            {date(2026, 7, 30), date(2026, 7, 31)},
        )
        # 8 月 1 日即使未被普通作息标成工作日，也应由系统总时长补足半天。
        august = schedule_context(2026, 8, set())

        july_result = calc_leave.calc_final_fields(row, july)
        august_result = calc_leave.calc_final_fields(row, august)

        self.assertEqual(july_result[:2], (16.0, 2.0))
        self.assertEqual(august_result[:2], (4.0, 0.5))
        self.assertEqual(july_result[0] + august_result[0], 20.0)
        self.assertEqual(july_result[2], "跨月按系统时长分摊")
        self.assertEqual(august_result[2], "跨月按系统时长分摊")

    def test_same_month_numeric_leave_keeps_existing_schedule_logic(self):
        row = (
            "MT0001",
            "测试员工",
            "总部",
            "产品部",
            "研发组",
            "年假",
            datetime(2026, 8, 3, 9, 0),
            datetime(2026, 8, 5, 18, 30),
            24,
        )
        august = schedule_context(
            2026,
            8,
            {date(2026, 8, 3), date(2026, 8, 4), date(2026, 8, 5)},
        )

        self.assertEqual(calc_leave.calc_final_fields(row, august), (24.0, 3.0, None))

    def test_year_boundary_keeps_system_total(self):
        row = (
            "MT0005",
            "测试员工",
            "总部",
            "产品部",
            "研发组",
            "年假",
            datetime(2026, 12, 31, 9, 0),
            datetime(2027, 1, 1, 12, 0),
            12,
        )
        december = schedule_context(2026, 12, {date(2026, 12, 31)})
        january = schedule_context(2027, 1, set())

        december_result = calc_leave.calc_final_fields(row, december)
        january_result = calc_leave.calc_final_fields(row, january)

        self.assertEqual(december_result[:2], (8.0, 1.0))
        self.assertEqual(january_result[:2], (4.0, 0.5))
        self.assertEqual(december_result[0] + january_result[0], 12.0)

    def test_natural_day_leave_keeps_calendar_day_semantics(self):
        row = (
            "MT0002",
            "测试员工",
            "总部",
            "产品部",
            "研发组",
            "陪产假",
            datetime(2026, 7, 31, 9, 0),
            datetime(2026, 8, 1, 18, 30),
            2,
        )
        july = schedule_context(2026, 7, set())
        august = schedule_context(2026, 8, set())

        self.assertEqual(calc_leave.calc_final_fields(row, july), (8, 1, None))
        self.assertEqual(calc_leave.calc_final_fields(row, august), (8, 1, None))

    def test_text_day_duration_does_not_enter_numeric_allocator(self):
        row = (
            "MT0003",
            "测试员工",
            "总部",
            "产品部",
            "研发组",
            "婚假",
            datetime(2026, 7, 30, 9, 0),
            datetime(2026, 8, 1, 18, 30),
            "3天",
        )
        july = schedule_context(
            2026,
            7,
            {date(2026, 7, 30), date(2026, 7, 31)},
        )

        result = calc_leave.calc_final_fields(row, july)
        self.assertEqual(result[:2], (16, 2))
        self.assertIsNone(result[2])

    def test_large_mismatch_falls_back_and_marks_manual_review(self):
        row = (
            "MT0004",
            "测试员工",
            "总部",
            "产品部",
            "研发组",
            "事假",
            datetime(2026, 7, 1, 9, 0),
            datetime(2026, 8, 31, 18, 30),
            8,
        )
        july = schedule_context(2026, 7, {date(2026, 7, 1)})

        result = calc_leave.calc_final_fields(row, july)
        self.assertEqual(result[:2], (8.0, 1.0))
        self.assertIn("需人工确认", result[2])

    def test_final_summary_day_details_reuse_cross_month_allocation(self):
        august = schedule_context(2026, 8, set())
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "请假明细表.xlsx"
            workbook = openpyxl.Workbook()
            worksheet = workbook.active
            worksheet.title = "请假明细"
            worksheet.append([
                "发起人工号", "发起人姓名", "一级部门", "二级部门", "三级部门",
                "请假类型", "开始时间", "结束时间", "系统时长", "最终请假时长",
                "最终请假天数", "备注",
            ])
            worksheet.append([
                "MT0040", "测试员工", "智慧零售", "网点拓展部", "区域",
                "事假", datetime(2026, 7, 30, 8, 0), datetime(2026, 8, 1, 12, 7),
                20, 4, 0.5, "跨月按系统时长分摊",
            ])
            workbook.save(path)
            workbook.close()

            details = calc_finally.parse_leave_day_details(str(path), august)

        self.assertEqual(details["MT0040"][date(2026, 8, 1)]["事假"], 0.5)


if __name__ == "__main__":
    unittest.main()
