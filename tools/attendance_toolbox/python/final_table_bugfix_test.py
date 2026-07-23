"""Regression tests for final-table data issues (leave/OT/subsidy path).

Covers the high-confidence code fixes for:
  - 3.5h triple-OT hour parsing and day conversion
  - revoked leave status filtering
  - 1-hour 调休 decimal precision
  - late22 exclusion for sales staff
  - RD subsidy keyword match across dept levels
  - resigned employee keep window for target-month history
"""
from __future__ import annotations

import os
import sys
import unittest
from unittest.mock import patch
from datetime import date
from pathlib import Path

BASE = Path(__file__).resolve().parent
for sub in ("leave", "overtime", "subsidy", "finally"):
    p = str(BASE / sub)
    if p not in sys.path:
        sys.path.insert(0, p)
if str(BASE) not in sys.path:
    sys.path.insert(0, str(BASE))

import calc_leave  # noqa: E402
import calc_finally as fin  # noqa: E402
import fill_overtime_fields as ot  # noqa: E402
import calc_subsidy_deduction as sub  # noqa: E402


class ParseFloatDurationTests(unittest.TestCase):
    def test_plain_number(self):
        self.assertEqual(ot.parse_float(3.5), 3.5)
        self.assertEqual(ot.parse_float("3.5"), 3.5)

    def test_hour_text(self):
        self.assertEqual(ot.parse_float("3.5小时"), 3.5)
        self.assertEqual(ot.parse_float("1 小时"), 1.0)
        self.assertEqual(ot.parse_float("0.5h"), 0.5)

    def test_day_text_converted_to_hours(self):
        self.assertEqual(ot.parse_float("0.5天"), 4.0)
        self.assertEqual(ot.parse_float("1天"), 8.0)

    def test_empty_and_invalid(self):
        self.assertIsNone(ot.parse_float(None))
        self.assertIsNone(ot.parse_float(""))
        self.assertIsNone(ot.parse_float("半天"))


class PremiumHourAndDayTests(unittest.TestCase):
    def test_3_5_hours_is_not_one_day(self):
        hours = ot._calc_premium_hour_value(3.5, None, 8.0)
        days = ot._calc_premium_days_from_hours(hours, 8.0)
        self.assertAlmostEqual(hours, 3.5, places=6)
        self.assertAlmostEqual(days, 3.5 / 8.0, places=6)
        self.assertNotEqual(days, 1.0)

    def test_one_hour_fraction(self):
        hours = ot._calc_premium_hour_value(1.0, None, 8.0)
        days = ot._calc_premium_days_from_hours(hours, 8.0)
        self.assertAlmostEqual(days, 0.125, places=6)

    def test_eight_hours_is_one_day(self):
        hours = ot._calc_premium_hour_value(8.0, None, 8.0)
        days = ot._calc_premium_days_from_hours(hours, 8.0)
        self.assertAlmostEqual(days, 1.0, places=6)

    def test_missing_raw_hours_does_not_default_to_full_day(self):
        # Previously raw_hours=None silently became standard_hours (8) → 1 day.
        hours = ot._calc_premium_hour_value(None, None, 8.0)
        self.assertEqual(hours, 0.0)
        days = ot._calc_premium_days_from_hours(hours, 8.0)
        self.assertEqual(days, 0.0)

    def test_hour_text_through_parse_float_pipeline(self):
        raw = ot.parse_float("3.5小时")
        hours = ot._calc_premium_hour_value(raw, None, 8.0)
        days = ot._calc_premium_days_from_hours(hours, 8.0)
        self.assertAlmostEqual(days, 0.4375, places=6)


class LeaveRevokeAndDurationTests(unittest.TestCase):
    def test_revoked_status_exact(self):
        self.assertTrue(calc_leave.is_revoked_approval("已撤销", None))
        self.assertTrue(calc_leave.is_revoked_approval("已撤销", "同意"))

    def test_revoked_status_keywords(self):
        self.assertTrue(calc_leave.is_revoked_approval("流程已终止", None))
        self.assertTrue(calc_leave.is_revoked_approval("作废", None))
        self.assertTrue(calc_leave.is_revoked_approval("完成", "申请人撤销"))

    def test_keep_valid_status(self):
        self.assertFalse(calc_leave.is_revoked_approval("完成", "同意"))
        self.assertFalse(calc_leave.is_revoked_approval("审批中", None))
        self.assertFalse(calc_leave.is_revoked_approval("已修改", "同意"))

    def test_one_hour_comp_leave_days_precision(self):
        # 1 hour 调休 must not collapse to 0 after hour→day conversion.
        final_h, final_days, _ = calc_leave.calc_from_system_duration(1.0)
        self.assertAlmostEqual(final_h, 1.0, places=2)
        self.assertGreater(final_days, 0)
        self.assertAlmostEqual(final_days, 0.13, places=2)  # ROUND_HALF_UP(0.125, 2)

    def test_parse_export_duration_hour_text(self):
        self.assertEqual(calc_leave.parse_export_duration("1小时"), 1.0)
        self.assertEqual(calc_leave.parse_export_duration("3.5小时"), 3.5)


class ResignKeepWindowTests(unittest.TestCase):
    def test_active_employee_kept(self):
        self.assertTrue(fin._in_resign_keep_window(None, 2026, 5))

    def test_resigned_during_target_month_kept(self):
        self.assertTrue(
            fin._in_resign_keep_window(date(2026, 5, 10), 2026, 5)
        )

    def test_resigned_after_target_month_kept(self):
        # Still employed throughout May → keep for May stats.
        self.assertTrue(
            fin._in_resign_keep_window(date(2026, 7, 1), 2026, 5)
        )

    def test_resigned_long_before_target_dropped(self):
        self.assertFalse(
            fin._in_resign_keep_window(date(2025, 12, 1), 2026, 5)
        )

    def test_resigned_previous_month_buffer_kept(self):
        self.assertTrue(
            fin._in_resign_keep_window(date(2026, 4, 20), 2026, 5)
        )


class OvertimeDaysFromHoursTests(unittest.TestCase):
    def test_finally_overtime_days_from_hours(self):
        self.assertAlmostEqual(fin._overtime_days_from_hours(3.5), 3.5 / 8.0, places=6)
        self.assertAlmostEqual(fin._overtime_days_from_hours(1.0), 0.125, places=6)
        self.assertEqual(fin._overtime_days_from_hours(0), 0.0)
        self.assertEqual(fin._overtime_days_from_hours(None), 0.0)


class Late22SalesExclusionTests(unittest.TestCase):
    def test_sales_dept_keyword_excluded(self):
        record = {
            "name": "李睿",
            "dept1": "AI智慧文创事业部",
            "dept2": "销售组",
            "dept3": None,
            "pos": "销售经理",
        }
        self.assertTrue(sub._should_exclude_late22_count(record, included_names=[]))

    def test_sales_position_only_excluded(self):
        record = {
            "name": "李睿",
            "dept1": "业务中心",
            "dept2": "华北区",
            "pos": "销售专员",
        }
        self.assertTrue(sub._should_exclude_late22_count(record, included_names=[]))

    def test_rd_not_excluded_by_sales_rule(self):
        record = {
            "name": "丁俊",
            "dept1": "智慧行政事业部",
            "dept2": "研发小组",
            "pos": "后端开发",
        }
        self.assertFalse(sub._should_exclude_late22_count(record, included_names=[]))

    def test_include_list_overrides(self):
        record = {
            "name": "崔利华",
            "dept1": "运营管理中心-运营支撑部",
            "pos": "",
        }
        self.assertFalse(
            sub._should_exclude_late22_count(record, included_names=["崔利华"])
        )


class RdDeptMatchTests(unittest.TestCase):
    def test_full_path_in_dept1(self):
        self.assertTrue(
            sub._is_rd_dept("智慧行政事业部-研发小组", keywords=sub.RD_DEPT_KEYWORDS)
        )

    def test_split_across_dept_levels(self):
        # Real bug: leaf group lives in dept2 while keyword is full path.
        self.assertTrue(
            sub._is_rd_dept(
                "智慧行政事业部",
                "研发小组",
                None,
                keywords=sub.RD_DEPT_KEYWORDS,
            )
        )

    def test_keyword_substring_match(self):
        self.assertTrue(
            sub._is_rd_dept("产品中心", keywords=sub.RD_DEPT_KEYWORDS)
        )
        self.assertTrue(
            sub._is_rd_dept("研发中心-平台组", keywords=sub.RD_DEPT_KEYWORDS)
        )

    def test_non_rd_excluded(self):
        self.assertFalse(
            sub._is_rd_dept(
                "AI智慧文创事业部",
                "销售组",
                None,
                keywords=sub.RD_DEPT_KEYWORDS,
            )
        )
        self.assertFalse(
            sub._is_rd_dept(
                "行政部",
                "前台",
                None,
                keywords=sub.RD_DEPT_KEYWORDS,
            )
        )

    def test_similar_but_not_rd(self):
        # 反例：名称相似但不在产研关键字列表
        self.assertFalse(
            sub._is_rd_dept(
                "智慧行政事业部",
                "综合支持组",
                None,
                keywords=sub.RD_DEPT_KEYWORDS,
            )
        )


class MaternityLeaveTypeTests(unittest.TestCase):
    def test_maternity_is_expected_attendance_day_leave(self):
        self.assertTrue(calc_leave.is_expected_attendance_day_leave("产假"))
        self.assertFalse(calc_leave.is_expected_attendance_day_leave("事假"))


class MaternityLeaveOverrideTests(unittest.TestCase):
    def setUp(self):
        expected_days = {
            date(2026, 6, 1), date(2026, 6, 2), date(2026, 6, 8),
            date(2026, 6, 10), date(2026, 6, 15), date(2026, 6, 16),
            date(2026, 6, 29),
        }
        working_days = expected_days - {date(2026, 6, 10)}
        self.schedule_ctx = {
            "year": 2026,
            "month": 6,
            "month_start": date(2026, 6, 1),
            "month_end": date(2026, 6, 30),
            "next_month_start": date(2026, 7, 1),
            "next_month_label": "七月考勤",
            "main_working_days": working_days,
            "chengdu_working_days": set(working_days),
            "main_expected_attendance_days": expected_days,
            "chengdu_expected_attendance_days": set(expected_days),
        }
        self.header = (
            "发起人工号", "发起人姓名", "一级部门", "二级部门", "三级部门",
            "请假类型", "开始时间", "结束时间", "系统时长", "发起时间",
            "完成时间", "审批编号", "审批状态", "审批结果", "是否实习生", "源文件行号",
        )

    def test_parse_json_and_optional_employee_number(self):
        parsed = calc_leave.parse_maternity_leave_overrides(
            '[{"name":"长期产假员工","start_date":"2026-05-20","end_date":"2026-07-10"}]'
        )
        self.assertEqual(parsed[0]["employee_no"], "")
        self.assertEqual(parsed[0]["start_date"], date(2026, 5, 20))
        self.assertEqual(parsed[0]["end_date"], date(2026, 7, 10))

    def test_add_when_dingtalk_has_no_maternity_leave(self):
        merged = calc_leave.merge_maternity_leave_overrides(
            [self.header],
            [{"name": "长期产假员工", "start_date": "2026-05-20", "end_date": "2026-07-10"}],
            self.schedule_ctx,
        )
        self.assertEqual(len(merged), 2)
        self.assertEqual(merged[1][calc_leave.ROW_LEAVE_TYPE], "产假")
        self.assertTrue(
            str(merged[1][calc_leave.ROW_APPROVAL_ID]).startswith(
                calc_leave.MATERNITY_OVERRIDE_APPROVAL_PREFIX
            )
        )

    def test_skip_when_dingtalk_maternity_leave_already_exists(self):
        existing = (
            "001", "长期产假员工", "", "", "", "产假",
            "2026-05-20 09:00", "2026-07-10 18:30", None,
            None, None, "DING-001", "完成", "同意", False, 2,
        )
        merged = calc_leave.merge_maternity_leave_overrides(
            [self.header, existing],
            [{"employee_no": "001", "name": "长期产假员工", "start_date": "2026-05-20", "end_date": "2026-07-10"}],
            self.schedule_ctx,
        )
        self.assertEqual(merged, [self.header, existing])

    def test_full_first_and_last_month_use_expected_attendance_dates(self):
        full = ("", "员工", "", "", "", "产假", "2026-05-01", "2026-07-31", None)
        first = ("", "员工", "", "", "", "产假", "2026-06-15", "2026-07-31", None)
        last = ("", "员工", "", "", "", "产假", "2026-05-01", "2026-06-10", None)
        self.assertEqual(calc_leave.calc_final_fields(full, self.schedule_ctx)[1], 7)
        self.assertEqual(calc_leave.calc_final_fields(first, self.schedule_ctx)[1], 3)
        self.assertEqual(calc_leave.calc_final_fields(last, self.schedule_ctx)[1], 4)

    def test_name_only_manual_leave_is_written(self):
        merged = calc_leave.merge_maternity_leave_overrides(
            [self.header],
            [{"name": "长期产假员工", "start_date": "2026-05-20", "end_date": "2026-07-10"}],
            self.schedule_ctx,
        )
        workbook = calc_leave.openpyxl.Workbook()
        with patch.object(calc_leave.openpyxl, "Workbook", return_value=workbook), patch.object(workbook, "save"):
            calc_leave.process(merged, "unused.xlsx", self.schedule_ctx)
        worksheet = workbook["请假明细"]
        self.assertEqual(worksheet.max_row, 2)
        self.assertEqual(worksheet.cell(2, 2).value, "长期产假员工")
        self.assertEqual(worksheet.cell(2, 12).value, "长期产假人工兜底")

    def test_maternity_day_detail_uses_expected_attendance_calendar(self):
        leave_rows = [{
            "emp_no": "",
            "name": "长期产假员工",
            "dept1": "",
            "dept2": "",
            "dept3": "",
            "leave_type": "产假",
            "raw_start": "2026-05-20 09:00",
            "raw_end": "2026-07-10 18:30",
        }]
        with patch.object(fin, "_collect_deduped_leave_rows", return_value=leave_rows):
            detail = fin.parse_leave_day_details("unused.xlsx", self.schedule_ctx)
        self.assertEqual(detail["长期产假员工"][date(2026, 6, 10)]["产假"], 1.0)


class EmploymentStatutoryBoundaryTests(unittest.TestCase):
    def setUp(self):
        self.month_start = date(2026, 6, 1)
        self.month_end = date(2026, 6, 30)

    def test_confirmation_day_same_as_statutory_holiday_is_counted(self):
        count = fin.calc_probation_days(
            date(2026, 6, 10),
            {date(2026, 6, 10), date(2026, 6, 11)},
            {date(2026, 6, 10)},
            self.month_start,
            self.month_end,
        )
        self.assertEqual(count, 2)

    def test_confirmation_day_same_as_weekend_statutory_holiday_is_counted(self):
        count = fin.calc_probation_days(
            date(2026, 6, 14),
            set(),
            {date(2026, 6, 14)},
            self.month_start,
            self.month_end,
        )
        self.assertEqual(count, 1)

    def test_statutory_holiday_before_confirmation_is_not_counted(self):
        count = fin.calc_probation_days(
            date(2026, 6, 11),
            set(),
            {date(2026, 6, 10)},
            self.month_start,
            self.month_end,
        )
        self.assertIsNone(count)

    def test_resignation_day_same_as_statutory_holiday_is_not_counted(self):
        count = fin.calc_statutory_holiday_days(
            None,
            date(2026, 6, 10),
            {date(2026, 6, 10)},
            self.month_start,
            self.month_end,
        )
        self.assertEqual(count, 0)

    def test_statutory_holiday_before_resignation_day_is_counted(self):
        count = fin.calc_statutory_holiday_days(
            None,
            date(2026, 6, 11),
            {date(2026, 6, 10), date(2026, 6, 11)},
            self.month_start,
            self.month_end,
        )
        self.assertEqual(count, 1)


class MaternityStatutoryHolidayTests(unittest.TestCase):
    def test_statutory_holiday_inside_maternity_is_removed(self):
        overlap = fin.calc_maternity_statutory_overlap_days(
            {
                date(2026, 6, 10): {"产假": 1.0},
                date(2026, 6, 11): {"事假": 1.0},
            },
            {date(2026, 6, 10), date(2026, 6, 11)},
            None,
            None,
            date(2026, 6, 1),
            date(2026, 6, 30),
        )
        self.assertEqual(overlap, 1)

    def test_statutory_holiday_outside_maternity_is_kept(self):
        overlap = fin.calc_maternity_statutory_overlap_days(
            {date(2026, 6, 9): {"产假": 1.0}},
            {date(2026, 6, 10)},
            None,
            None,
            date(2026, 6, 1),
            date(2026, 6, 30),
        )
        self.assertEqual(overlap, 0)


if __name__ == "__main__":
    unittest.main()
