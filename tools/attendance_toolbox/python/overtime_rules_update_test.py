from datetime import date, datetime
from pathlib import Path
import sys
import tempfile
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
        for status, result in (
            ("草稿", "同意"),
            ("完成", ""),
            ("完成", "拒绝"),
            ("完成", "撤回"),
            ("完成", "作废"),
        ):
            with self.subTest(status=status, result=result):
                self.assertFalse(ot._is_effective_approval_status(status, result))

    def test_clean_export_filters_invalid_approval_and_does_not_repeat_daily_hours(self):
        import openpyxl

        headers = [
            "审批状态", "审批结果", "审批编号", "发起人工号", "发起人姓名", "发起人部门",
            "开始时间", "结束时间", "明细", "加班时间", "加班时长", "时长",
        ]
        with tempfile.TemporaryDirectory() as temp_dir:
            path = Path(temp_dir) / "overtime.xlsx"
            wb = openpyxl.Workbook()
            ws = wb.active
            ws.append(headers)
            ws.append([
                "完成", "同意", "A1", "MT-1", "员工", "总部-部门-小组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23), None, None, None, 10,
            ])
            ws.append([
                "完成", "作废", "A2", "MT-2", "作废员工", "总部-部门-小组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23), None, None, None, 8,
            ])
            wb.save(path)
            wb.close()

            rows = ot.clean_export_overtime(str(path))

        self.assertEqual(len(rows), 3)
        self.assertEqual(rows[1][1], "员工")
        self.assertEqual(rows[2][1], "员工")
        self.assertIsNone(rows[1][9])
        self.assertIsNone(rows[2][9])

    def test_approval_duration_has_priority_over_bilateral_punch(self):
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
        self.assertEqual(ws.cell(header_row + 1, header_map["最终加班时长（小时）"]).value, 2)
        self.assertIn("审批时长与打卡时长不一致", ws.cell(header_row + 1, header_map["备注"]).value)

    def test_daily_approval_details_are_used_for_cross_day_rows(self):
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 15), None, 3.0, 10.0, None, None,
            ],
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 16, 23),
                date(2026, 6, 16), None, 4.0, 10.0, None, None,
            ],
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        contexts = ot._build_general_overtime_contexts(
            ws, header_map, [header_row + 1, header_row + 2], None, get_default_config()
        )
        self.assertEqual(contexts[header_row + 1]["final_hours"], 3)
        self.assertEqual(contexts[header_row + 2]["final_hours"], 4)

    def test_manual_calculation_zero_is_preserved(self):
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, None, None, None, None,
            ]
        )
        ws.cell(1, ws.max_column + 1).value = "人工核算结果"
        ws.cell(2, ws.max_column).value = 0
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        contexts = ot._build_general_overtime_contexts(
            ws, header_map, [header_row + 1], None, get_default_config()
        )
        self.assertEqual(contexts[header_row + 1]["final_hours"], 0)

    def test_no_overtime_evidence_is_not_written_as_zero(self):
        ws = self._workbook(
            [
                "MT-OT", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, None, None, None, None,
            ]
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        contexts = ot._build_general_overtime_contexts(
            ws, header_map, [header_row + 1], None, get_default_config()
        )
        self.assertIsNone(contexts[header_row + 1]["final_hours"])

    def test_schedule_only_row_is_marked_without_pay_or_comp_time(self):
        ot._SCHEDULE_ONLY_ROW_KEYS.add(
            ("员工", date(2026, 6, 15), "2026-06-15T18:00:00", "2026-06-15T23:00:00")
        )
        ws = self._workbook(
            [
                "", "员工", "总部", "普通部门", "普通组",
                datetime(2026, 6, 15, 18), datetime(2026, 6, 15, 23),
                date(2026, 6, 15), None, None, 8.0, None, None,
            ]
        )
        header_row = ot.find_header_row(ws)
        header_map = ot.ensure_target_columns(ws, header_row, ot.build_header_map(ws, header_row))
        result = ot.fill_row(
            ws, header_row + 1, header_map, {}, date(2026, 6, 1), None,
            get_default_config(), schedule_only_row_indices={header_row + 1},
        )
        self.assertEqual(result, "疑似漏报")
        self.assertIsNone(ws.cell(header_row + 1, header_map["最终加班时长（小时）"]).value)
        self.assertEqual(ws.cell(header_row + 1, header_map["系统操作"]).value, "疑似漏报")
        ot._SCHEDULE_ONLY_ROW_KEYS.clear()

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

    def test_ops_group_is_not_excluded_and_keeps_recorded_manual_override(self):
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
        self.assertIn(header_row + 1, contexts)
        self.assertEqual(contexts[header_row + 1]["basis"], "manual")
        self.assertEqual(contexts[header_row + 1]["final_hours"], 6)

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
