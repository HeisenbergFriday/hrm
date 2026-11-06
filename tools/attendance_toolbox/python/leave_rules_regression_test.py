from __future__ import annotations

import contextlib
import io
import tempfile
import unittest
from datetime import date, datetime
from pathlib import Path

import openpyxl

from leave import calc_leave
from cross_month_leave_test import schedule_context


class LeaveRulesRegressionTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.ctx = schedule_context(2026, 9, {date(2026, 9, day) for day in range(1, 31)})

    def row(self, kind, hours, approval="A1"):
        return (
            "MT1", "测试员工", "总部", "部门", "小组", kind,
            datetime(2026, 9, 1, 9), datetime(2026, 9, 3, 18, 30), hours,
            datetime(2026, 8, 25, 9), datetime(2026, 8, 26, 9), approval,
            "完成", "同意", False, 2, "", "", "",
        )

    def test_maternity_is_natural_days(self):
        output = Path(self.tmp.name) / "leave.xlsx"
        header = (
            "发起人工号", "发起人姓名", "一级部门", "二级部门", "三级部门", "请假类型",
            "开始时间", "结束时间", "系统时长", "发起时间", "完成时间", "审批编号",
            "审批状态", "审批结果", "是否实习生", "源文件行号", "审批实例ID", "审批流程码",
            "发起人钉钉UserID",
        )
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process([header, self.row("产假", "3天")], str(output), self.ctx)
        wb = openpyxl.load_workbook(output, data_only=True)
        try:
            ws = wb["请假明细"]
            columns = {cell.value: cell.column for cell in ws[1]}
            self.assertEqual(ws.cell(2, columns["计量口径"]).value, "自然日")
            self.assertEqual(ws.cell(2, columns["最终请假天数"]).value, 3)
            self.assertIsNone(ws.cell(2, columns["最终请假时长"]).value)
        finally:
            wb.close()

    def test_previous_confirmations_survive_regeneration(self):
        first = Path(self.tmp.name) / "first.xlsx"
        second = Path(self.tmp.name) / "second.xlsx"
        header = (
            "发起人工号", "发起人姓名", "一级部门", "二级部门", "三级部门", "请假类型",
            "开始时间", "结束时间", "系统时长", "发起时间", "完成时间", "审批编号",
            "审批状态", "审批结果", "是否实习生", "源文件行号", "审批实例ID", "审批流程码",
            "发起人钉钉UserID",
        )
        rows = [header, self.row("事假", 4)]
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process(rows, str(first), self.ctx)
        wb = openpyxl.load_workbook(first)
        ws = wb["请假明细"]
        columns = {cell.value: cell.column for cell in ws[1]}
        ws.cell(2, columns["复核状态"], "已确认")
        ws.cell(2, columns["确认请假时长"], 4)
        ws.cell(2, columns["确认说明"], "人事已核对")
        wb.save(first)
        wb.close()
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process(rows, str(second), self.ctx, reviewed_file=str(first))
        wb = openpyxl.load_workbook(second, data_only=True)
        try:
            ws = wb["请假明细"]
            columns = {cell.value: cell.column for cell in ws[1]}
            self.assertEqual(ws.cell(2, columns["复核状态"]).value, "已确认")
            self.assertEqual(ws.cell(2, columns["确认请假时长"]).value, 4)
            self.assertEqual(ws.cell(2, columns["确认说明"]).value, "人事已核对")
            self.assertEqual(wb["异常清单"].max_row, 1)
        finally:
            wb.close()


if __name__ == "__main__":
    unittest.main()
