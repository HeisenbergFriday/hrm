from datetime import date, datetime
import unittest

from overtime import fill_overtime_fields as overtime
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


if __name__ == "__main__":
    unittest.main()
