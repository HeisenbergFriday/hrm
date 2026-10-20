from datetime import date, datetime
from pathlib import Path
import sys
import unittest


HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from overtime import fill_overtime_fields as ot
from overtime.rules_engine import get_default_config


class OvertimeRuleUpdateTests(unittest.TestCase):
    def test_only_effective_approval_statuses_are_calculated(self):
        self.assertTrue(ot._is_effective_approval_status("完成", "同意"))
        self.assertTrue(ot._is_effective_approval_status("审批中", ""))
        self.assertTrue(ot._is_effective_approval_status("已修改", "通过"))
        for status, result in (("草稿", "同意"), ("完成", "拒绝"), ("完成", "撤销")):
            with self.subTest(status=status, result=result):
                self.assertFalse(ot._is_effective_approval_status(status, result))

    def test_bilateral_punch_has_priority_over_system_duration(self):
        ot._reset_attendance_runtime_state()
        ot._ATTENDANCE_CLOCK_HOURS_BY_CODE["MT-OT"] = {date(2026, 6, 15): 5.5}
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, None, 2.0, None, None,
            ]
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        config = get_default_config()
        contexts = ot._build_general_overtime_contexts(ws, header_map, [header_row + 1], None, config)
        ot.fill_row(ws, header_row + 1, header_map, {}, date(2026, 6, 1), None, config, general_overtime_contexts=contexts)
        self.assertEqual(ws.cell(header_row + 1, header_map["最终加班时长（小时）"]).value, 5.5)

    def test_missing_bilateral_punch_falls_back_to_system_duration(self):
        ot._reset_attendance_runtime_state()
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, None, 6.0, None, None,
            ]
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        config = get_default_config()
        contexts = ot._build_general_overtime_contexts(ws, header_map, [header_row + 1], None, config)
        ot.fill_row(ws, header_row + 1, header_map, {}, date(2026, 6, 1), None, config, general_overtime_contexts=contexts)
        self.assertEqual(ws.cell(header_row + 1, header_map["最终加班时长（小时）"]).value, 6)

    def test_cross_day_rows_share_approval_total_cap(self):
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 15), None, 8.0, 10.0, None, None,
            ],
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 16), None, 8.0, 10.0, None, None,
            ],
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        contexts = ot._build_general_overtime_contexts(
            ws, header_map, [header_row + 1, header_row + 2], None, get_default_config()
        )
        self.assertEqual(sum(item["final_hours"] for item in contexts.values()), 10.0)
        self.assertLessEqual(contexts[header_row + 1]["final_hours"], 8.0)
        self.assertLessEqual(contexts[header_row + 2]["final_hours"], 8.0)

    def test_over_total_requires_recorded_manual_override(self):
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 15), None, 8.0, 10.0, None, None,
            ],
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 16), None, 8.0, 10.0, None, None,
            ],
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        ws.cell(header_row + 1, header_map["人工覆盖时长（小时）"]).value = 8
        ws.cell(header_row + 1, header_map["人工覆盖说明"]).value = "负责人确认跨天实际加班"
        config = get_default_config()
        contexts = ot._build_general_overtime_contexts(ws, header_map, [header_row + 1, header_row + 2], None, config)
        self.assertEqual(contexts[header_row + 1]["basis"], "manual")
        self.assertEqual(contexts[header_row + 1]["final_hours"], 8)
        self.assertEqual(sum(item["final_hours"] for item in contexts.values()), 10)

    def test_excluded_ops_group_stays_unadded_even_with_manual_override(self):
        ws = self._workbook(
            [
                "MT-OPS", "\u8fd0\u7ef4\u5458\u5de5", "\u603b\u90e8\u95e8", "\u8fd0\u8425\u652f\u6301\u90e8", "\u8fd0\u7ef4\u7ec4",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, 6.0, 6.0, None, None,
            ]
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        ws.cell(header_row + 1, header_map["\u4eba\u5de5\u8986\u76d6\u65f6\u957f\uff08\u5c0f\u65f6\uff09"]).value = 6
        ws.cell(header_row + 1, header_map["\u4eba\u5de5\u8986\u76d6\u8bf4\u660e"]).value = "\u4e3b\u7ba1\u786e\u8ba4"
        contexts = ot._build_general_overtime_contexts(
            ws, header_map, [header_row + 1], None, get_default_config()
        )
        self.assertNotIn(header_row + 1, contexts)

    def _workbook(self, *rows):
        import openpyxl

        wb = openpyxl.Workbook()
        ws = wb.active
        ws.title = "加班明细"
        ws.append(list(ot.CLEAN_OUTPUT_COLS))
        for row in rows:
            ws.append(row)
        return ws


if __name__ == "__main__":
    unittest.main()
