# -*- coding: utf-8 -*-
from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

import openpyxl

MODULE_PATH = Path(__file__).resolve().parent / "subsidy" / "calc_subsidy_deduction.py"


def _load_module():
    spec = importlib.util.spec_from_file_location("calc_subsidy_deduction_all_people_test", MODULE_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


class SubsidyAllPeopleMonthlySummaryTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.module = _load_module()

    def _write_workbook(self, path: Path, *, all_people: bool) -> None:
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.title = "月度汇总"
        ws["A1"] = "月度汇总表（补贴及扣款） 统计日期：2026-07-01 至 2026-07-01" if all_people else "旧格式月度汇总"
        headers = [
            "姓名", "考勤组", "部门" if all_people else "一级部门", "工号", "职位",
            "UserId" if all_people else "备注", "入职时间", "休息日&节假日加班打卡时长",
            "本月加班审批流时长", "本月加班时长（工作日加班）", "15分钟内迟到次数",
            "15分钟内迟到分钟数", "15-30分钟迟到次数", "15-30分钟迟到分钟数",
            "超30分钟迟到次数", "超30分钟迟到分钟数", "迟到总次数", "迟到总时长(分钟)",
            "15分钟内早退次数", "15分钟内早退分钟数", "15-30分钟早退次数",
            "15-30分钟早退分钟数", "超30分钟早退次数", "超30分钟早退分钟数",
            "早退总次数", "早退总时长(分钟)", "15-30分钟迟到扣款", "15-30分钟早退扣款",
            "旷工天数", "缺卡次数", "晚于22点打卡天数", "晚走补贴",
            "产研休息日加班>4小时天数", "产研休息日加班补贴", "1", "2", "3", "六", "日",
        ]
        for col, value in enumerate(headers, 1):
            ws.cell(4, col, value)
        rows = [
            ["空工号", "未加入考勤组", "研发部", None, "工程师", "ding-empty", "2026-01-01"],
            ["数字工号", "全员", "支持部", 7, "专员", "ding-seven", "2026-01-01"],
            ["标准工号", "全员", "研发部", "MT0001", "工程师", "ding-mt", "2026-01-01"],
        ]
        for row_index, values in enumerate(rows, 5):
            for col, value in enumerate(values, 1):
                ws.cell(row_index, col, value)
            for col in range(35, 40):
                ws.cell(row_index, col, "标准:正常\n(09:00,18:30)")
        wb.save(path)
        wb.close()

    def test_all_people_table_keeps_empty_and_special_employee_numbers(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "all_people.xlsx"
            self._write_workbook(path, all_people=True)
            source = self.module.parse_source_table(str(path))
            attendance = self.module.parse_attendance(str(path), year=2026, month=7)
            self.assertEqual([row["name"] for row in source], ["空工号", "数字工号", "标准工号"])
            self.assertEqual([row["name"] for row in attendance], ["空工号", "数字工号", "标准工号"])
            self.assertEqual(source[0]["dept1"], "研发部")
            self.assertIsNone(source[0]["dept2"])
            self.assertEqual(attendance[0]["pos"], "工程师")

    def test_old_table_still_filters_nonstandard_employee_numbers(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "legacy.xlsx"
            self._write_workbook(path, all_people=False)
            source = self.module.parse_source_table(str(path))
            attendance = self.module.parse_attendance(str(path), year=2026, month=7)
            self.assertEqual([row["name"] for row in source], ["标准工号"])
            self.assertEqual([row["name"] for row in attendance], ["标准工号"])


if __name__ == "__main__":
    unittest.main()
