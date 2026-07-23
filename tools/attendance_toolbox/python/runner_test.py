from datetime import date, datetime
import unittest

from openpyxl import Workbook
from overtime import fill_overtime_fields as overtime
from subsidy import calc_subsidy_deduction as subsidy
import runner


class RunnerNameParsingTest(unittest.TestCase):
    def test_names_accepts_display_separators(self):
        self.assertEqual(
            runner.names({"names": "梁伯林、陈秋宇，蔡依诺,王天钦\n周灵京；秦洋"}, "names"),
            ["梁伯林", "陈秋宇", "蔡依诺", "王天钦", "周灵京", "秦洋"],
        )

    def test_names_accepts_form_value_list(self):
        self.assertEqual(
            runner.names({"names": ["费婷玉、张莹", "陈星雨，伏鸣"]}, "names"),
            ["费婷玉", "张莹", "陈星雨", "伏鸣"],
        )


class OvertimeValidationTest(unittest.TestCase):
    def test_holiday_validation_ignores_rows_outside_target_month(self):
        old_row = [None] * len(overtime.CLEAN_OUTPUT_COLS)
        old_row[5] = datetime(2025, 10, 1, 9, 0)
        old_row[6] = datetime(2025, 10, 1, 18, 0)
        old_row[7] = date(2025, 10, 1)

        target_row = [None] * len(overtime.CLEAN_OUTPUT_COLS)
        target_row[5] = datetime(2026, 4, 1, 9, 0)
        target_row[6] = datetime(2026, 4, 1, 18, 0)
        target_row[7] = date(2026, 4, 1)

        filtered_rows, skipped = overtime.filter_rows_by_target_month(
            [overtime.CLEAN_OUTPUT_COLS, tuple(old_row), tuple(target_row)],
            date(2026, 4, 1),
        )

        self.assertEqual(skipped, 1)
        self.assertEqual(overtime._collect_holiday_years(filtered_rows), {2026})


class ToolboxMonthAndHolidayLinkageTest(unittest.TestCase):
    def test_parse_target_month_accepts_month_and_date(self):
        self.assertEqual(runner.parse_target_month("2026-07"), (2026, 7))
        self.assertEqual(runner.parse_target_month("2026/07/18"), (2026, 7))
        self.assertIsNone(runner.parse_target_month(""))

    def test_parse_target_month_rejects_invalid_value(self):
        with self.assertRaisesRegex(ValueError, "YYYY-MM"):
            runner.parse_target_month("2026-13", "补贴考勤月份")

    def test_locked_month_rejects_schedule_conflict(self):
        with self.assertRaisesRegex(ValueError, "但作息表识别为 2026年6月"):
            runner.validate_target_month_match(
                "2026-07",
                (2026, 6),
                label="补贴考勤月份",
            )

    def test_custom_holidays_override_schedule_and_record_conflicts(self):
        selected, audit = runner.select_subsidy_legal_holidays(
            {date(2026, 7, 1), date(2026, 7, 2)},
            {date(2026, 7, 1), date(2026, 7, 3), date(2025, 10, 1)},
            2026,
            7,
        )
        self.assertEqual(selected, {date(2026, 7, 1), date(2026, 7, 3)})
        self.assertEqual(audit["source"], "custom_rules")
        self.assertEqual(audit["only_in_rules"], ["2026-07-03"])
        self.assertEqual(audit["only_in_schedule"], ["2026-07-02"])
        self.assertEqual(audit["conflict_count"], 2)

    def test_schedule_holidays_are_used_without_override(self):
        selected, audit = runner.select_subsidy_legal_holidays(
            {date(2026, 7, 1)},
            set(),
            2026,
            7,
        )
        self.assertEqual(selected, {date(2026, 7, 1)})
        self.assertEqual(audit["source"], "schedule")
        self.assertEqual(audit["conflict_count"], 0)

    def test_exception_audit_sheet_lists_missing_attendance(self):
        wb = Workbook()
        subsidy.append_exception_audit_sheet(
            wb,
            missing_attendance=["李睿", "李睿", "任澳辉"],
            intern_names=["实习员工"],
            audit_context={
                "target_month": "2026-07",
                "holiday_source": "custom_rules",
                "holiday_count": 2,
                "holiday_conflict_count": 1,
                "holiday_only_in_rules": ["2026-07-03"],
            },
        )
        ws = wb["异常审计"]
        values = [tuple(cell.value for cell in row) for row in ws.iter_rows()]
        flattened = "\n".join(str(item) for row in values for item in row if item is not None)
        self.assertIn("李睿", flattened)
        self.assertIn("任澳辉", flattened)
        self.assertIn("2026-07-03", flattened)
        self.assertEqual(sum(1 for row in values if row[1] == "李睿"), 1)

    def test_audit_meta_keeps_missing_names_and_counts(self):
        meta = runner.build_subsidy_audit_meta(
            year=2026,
            month=7,
            holiday_audit={"source": "schedule", "selected_count": 1, "conflict_count": 0},
            missing_attendance=["丁俊", "丁俊", "任澳辉"],
            intern_names=[],
        )
        audit = meta["subsidy_audit"]
        self.assertEqual(audit["missing_attendance_count"], 2)
        self.assertEqual(audit["missing_attendance_names"], ["丁俊", "任澳辉"])


if __name__ == "__main__":
    unittest.main()
