"""Exercise the Excel review workflow from source export through final parsing."""
from __future__ import annotations

import contextlib
import io
import json
import sys
import tempfile
import unittest
from datetime import date, datetime, timedelta
from pathlib import Path

import openpyxl

BASE = Path(__file__).resolve().parent
for directory in (BASE / "leave", BASE / "finally"):
    sys.path.insert(0, str(directory))
import calc_leave
import calc_finally
from cross_month_leave_test import schedule_context


class LeaveReviewWorkflowTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.path = Path(self.tmp.name) / "请假明细表.xlsx"
        self.ctx = schedule_context(2026, 9, {date(2026, 9, day) for day in (1, 2, 3, 4)})

    def row(
        self,
        hours=8,
        *,
        approval="A1",
        kind="事假",
        start=None,
        end=None,
        status="完成",
        emp_id="MT0001",
        name="测试员工",
        instance_id="",
        process_code="",
        originator_userid="",
    ):
        return (
            emp_id, name, "总部", "产品部", "研发组", kind,
            start or datetime(2026, 9, 1, 9), end or datetime(2026, 9, 1, 18, 30),
            hours, datetime(2026, 8, 25, 9), datetime(2026, 8, 26, 9),
            approval, status, "同意", False, 2,
            instance_id, process_code, originator_userid,
        )

    def generate(self, rows):
        header = (
            "发起人工号", "发起人姓名", "一级部门", "二级部门", "三级部门", "请假类型",
            "开始时间", "结束时间", "系统时长", "发起时间", "完成时间", "审批编号",
            "审批状态", "审批结果", "是否实习生", "源文件行号",
            "审批实例ID", "审批流程码", "发起人钉钉UserID",
        )
        self.source_rows = [header] + rows
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process(self.source_rows, str(self.path), self.ctx)

    def edit(self, changes, row=2):
        wb = openpyxl.load_workbook(self.path)
        ws = wb["请假明细"]
        headers = {cell.value: cell.column for cell in ws[1]}
        for key, value in changes.items():
            ws.cell(row, headers[key], value)
        wb.save(self.path)
        wb.close()

    def values(self):
        wb = openpyxl.load_workbook(self.path, data_only=True)
        ws = wb["请假明细"]
        data = dict(zip((cell.value for cell in ws[1]), (cell.value for cell in ws[2])))
        count = wb["异常清单"].max_row - 1
        wb.close()
        return data, count

    def test_full_day_duration_is_eight_hours_at_review_entry(self):
        result = calc_leave.review_leave_record(
            self.row(
                8,
                start=datetime(2026, 9, 1, 9, 30),
                end=datetime(2026, 9, 1, 18, 30),
            ),
            self.ctx,
        )
        self.assertEqual(result[0], 8.0)
        self.assertEqual(result[1], 1.0)
        self.assertEqual(result[2]["2026-09-01"]["hours"], 8.0)

    def test_fractional_hours_round_up_to_half_hour_at_review_entry(self):
        result = calc_leave.review_leave_record(
            self.row(
                1.17,
                start=datetime(2026, 9, 1, 10, 20),
                end=datetime(2026, 9, 1, 11, 30),
            ),
            self.ctx,
        )
        self.assertEqual(result[0], 1.5)
        self.assertEqual(result[1], 0.19)
        self.assertEqual(result[2]["2026-09-01"]["hours"], 1.5)

    def test_matching_same_month_is_automatic_and_ledger_is_shared(self):
        self.generate([self.row()])
        values, issues = self.values()
        self.assertEqual(values["复核状态"], "自动通过")
        self.assertEqual(issues, 0)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["事假"], 1)
        details = calc_finally.parse_leave_day_details(str(self.path), self.ctx)
        self.assertEqual(details["MT0001"][date(2026, 9, 1)]["事假"], 1)

    def test_maternity_leave_uses_calendar_days_for_same_and_cross_month(self):
        rows = [
            self.row(
                8,
                kind="产假",
                emp_id="MT-YM",
                name="银梦",
                start=datetime(2026, 9, 14, 9),
                end=datetime(2026, 9, 28, 18, 30),
                approval="MAT-YM",
            ),
            self.row(
                8,
                kind="产假",
                emp_id="MT-WSL",
                name="王思铃",
                start=datetime(2026, 8, 18, 9),
                end=datetime(2026, 9, 1, 18, 30),
                approval="MAT-WSL",
            ),
        ]
        self.generate(rows)

        workbook = openpyxl.load_workbook(self.path, data_only=True)
        try:
            worksheet = workbook["请假明细"]
            columns = {cell.value: cell.column for cell in worksheet[1]}
            by_name = {
                worksheet.cell(row, columns["发起人姓名"]).value: {
                    "days": worksheet.cell(row, columns["最终请假天数"]).value,
                    "ledger": json.loads(worksheet.cell(row, columns["日分摊"]).value),
                    "state": worksheet.cell(row, columns["复核状态"]).value,
                }
                for row in range(2, worksheet.max_row + 1)
            }
        finally:
            workbook.close()

        self.assertEqual(by_name["银梦"]["days"], 15)
        self.assertEqual(len(by_name["银梦"]["ledger"]), 15)
        self.assertEqual(by_name["王思铃"]["days"], 1)
        self.assertEqual(
            by_name["王思铃"]["ledger"],
            {"2026-09-01": {"days": 1.0, "hours": None}},
        )
        self.assertEqual({item["state"] for item in by_name.values()}, {"自动通过"})

        summary = calc_finally.parse_leave_summary(str(self.path), self.ctx)
        self.assertEqual(summary["MT-YM"]["产假"], 15)
        self.assertEqual(summary["MT-WSL"]["产假"], 1)

    def test_maternity_type_with_detail_suffix_keeps_natural_day_unit(self):
        self.assertTrue(calc_leave.is_natural_day_leave("产假（顺延）"))
        self.assertFalse(calc_leave.is_natural_day_leave("产检假"))

    def test_mismatch_uses_schedule_result_without_exception(self):
        self.generate([self.row(4)])
        values, issues = self.values()
        self.assertEqual(values["系统时长"], 4)
        self.assertEqual(values["作息时长"], 8)
        self.assertEqual(values["最终请假时长"], 8)
        self.assertEqual(values["复核状态"], "自动通过")
        self.assertEqual(issues, 0)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["事假"], 1)

    def test_confirmation_changes_both_summary_and_daily_detail(self):
        self.generate([self.row(4)])
        self.edit({"复核状态": "已确认", "确认请假时长": 4, "确认说明": "核对审批，实际请半天"})
        regenerated = Path(self.tmp.name) / "regenerated.xlsx"
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process(self.source_rows, str(regenerated), self.ctx, reviewed_file=str(self.path))
        summary = calc_finally.parse_leave_summary(str(regenerated), self.ctx)
        detail = calc_finally.parse_leave_day_details(str(regenerated), self.ctx)
        self.assertEqual(summary["MT0001"]["事假"], 0.5)
        self.assertEqual(detail["MT0001"][date(2026, 9, 1)]["事假"], 0.5)

    def test_zero_confirmation_does_not_fall_back_to_system(self):
        self.generate([self.row(4)])
        self.edit({"复核状态": "已确认", "确认请假时长": 0, "确认说明": "重复申请，本月不入账"})
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx), {})
        self.assertEqual(calc_finally.parse_leave_day_details(str(self.path), self.ctx), {})

    def test_final_amount_entry_is_reused_when_explicitly_confirmed(self):
        self.generate([self.row(4)])
        self.edit({"复核状态": "已确认", "最终请假时长": 4, "确认说明": "人事已核对"})
        regenerated = Path(self.tmp.name) / "regenerated-final.xlsx"
        with contextlib.redirect_stdout(io.StringIO()):
            calc_leave.process(self.source_rows, str(regenerated), self.ctx, reviewed_file=str(self.path))
        workbook = openpyxl.load_workbook(regenerated, data_only=True)
        worksheet = workbook["请假明细"]
        columns = {cell.value: cell.column for cell in worksheet[1]}
        self.assertEqual(worksheet.cell(2, columns["确认请假时长"]).value, 4)
        self.assertEqual(worksheet.cell(2, columns["最终请假时长"]).value, 4)
        self.assertEqual(worksheet.cell(2, columns["复核状态"]).value, "已确认")
        workbook.close()
        rows = calc_finally._collect_deduped_leave_rows(str(regenerated))
        self.assertEqual(rows[0]["raw_days"], 0.5)
        summary = calc_finally.parse_leave_summary(str(regenerated), self.ctx)
        self.assertEqual(summary["MT0001"]["事假"], 0.5)

    def test_state_only_and_missing_reason_cannot_bypass_review(self):
        self.generate([self.row(4)])
        self.edit({"复核状态": "已确认"})
        with self.assertRaisesRegex(ValueError, "确认说明"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)
        self.edit({"确认说明": "已核对"})
        with self.assertRaisesRegex(ValueError, "确认值"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_bad_system_values_use_calculable_schedule_result(self):
        for amount in (None, "", "坏数据", "3..5小时", "3..5天", -1, "NaN", "inf", 80):
            with self.subTest(amount=amount):
                self.generate([self.row(amount)])
                values, issues = self.values()
                self.assertEqual(values["复核状态"], "自动通过")
                self.assertEqual(values["最终请假时长"], 8)
                self.assertEqual(issues, 0)

    def test_missing_ids_do_not_collapse_multiple_source_rows(self):
        path = Path(self.tmp.name) / "source.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append(["审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门", "请假类型", "开始时间", "结束时间", "时长", "审批编号"])
        for person in ("MT1", "MT2"):
            ws.append(["完成", "同意", person, person, "总部", "事假", "2026-09-01", "2026-09-01", "8小时", None])
        wb.save(path)
        wb.close()
        with contextlib.redirect_stdout(io.StringIO()):
            rows = calc_leave.clean_export(str(path))
        self.assertEqual(len(rows), 3)

    def test_latest_revocation_invalidates_old_approved_version(self):
        path = Path(self.tmp.name) / "source.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append(["审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门", "请假类型", "开始时间", "结束时间", "时长", "审批编号", "完成时间"])
        for status, finish in (("已撤销", "2026-09-03 12:00"), ("完成", "2026-09-02 12:00")):
            ws.append([status, "同意", "MT1", "员工", "总部", "事假", "2026-09-01", "2026-09-01", "8小时", "A1", finish])
        wb.save(path)
        wb.close()
        with contextlib.redirect_stdout(io.StringIO()):
            rows = calc_leave.clean_export(str(path))
        self.assertEqual(len(rows), 1)

    def test_same_business_id_different_originators_are_kept_end_to_end(self):
        path = Path(self.tmp.name) / "source-with-identity.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append([
            "审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门",
            "请假类型", "开始时间", "结束时间", "时长", "审批编号",
            "发起时间", "完成时间", "审批实例ID", "审批流程码", "发起人钉钉UserID",
        ])
        for emp_id, name, instance_id, originator in (
            ("MT1", "员工甲", "pi-1", "ding-1"),
            ("MT2", "员工乙", "pi-2", "ding-2"),
        ):
            ws.append([
                "完成", "同意", emp_id, name, "总部", "事假",
                datetime(2026, 9, 1, 9), datetime(2026, 9, 1, 18, 30), "8小时", "BIZ-1",
                datetime(2026, 8, 25, 9), datetime(2026, 8, 26, 9),
                instance_id, "PROC-LEAVE", originator,
            ])
        wb.save(path)
        wb.close()

        with contextlib.redirect_stdout(io.StringIO()) as stdout:
            rows = calc_leave.clean_export(str(path))
            calc_leave.process(rows, str(self.path), self.ctx)
        self.assertEqual(3, len(rows))
        self.assertIn("跨发起人复用", stdout.getvalue())
        summary = calc_finally.parse_leave_summary(str(self.path), self.ctx)
        self.assertEqual(1, summary["MT1"]["事假"])
        self.assertEqual(1, summary["MT2"]["事假"])
        result = openpyxl.load_workbook(self.path, data_only=True)
        try:
            sheet = result["请假明细"]
            columns = {cell.value: cell.column for cell in sheet[1]}
            self.assertTrue(sheet.column_dimensions[sheet.cell(1, columns["审批实例ID"]).column_letter].hidden)
        finally:
            result.close()

    def test_same_originator_business_id_still_selects_latest_revocation(self):
        path = Path(self.tmp.name) / "source-versioned.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append([
            "审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门",
            "请假类型", "开始时间", "结束时间", "时长", "审批编号", "完成时间",
            "审批实例ID", "审批流程码", "发起人钉钉UserID",
        ])
        for status, finish, instance_id in (
            ("完成", "2026-09-02 12:00", "pi-old"),
            ("已撤销", "2026-09-03 12:00", "pi-new"),
        ):
            ws.append([
                status, "同意", "MT1", "员工", "总部", "事假",
                "2026-09-01", "2026-09-01", "8小时", "BIZ-1", finish,
                instance_id, "PROC-LEAVE", "ding-1",
            ])
        wb.save(path)
        wb.close()
        with contextlib.redirect_stdout(io.StringIO()):
            rows = calc_leave.clean_export(str(path))
        self.assertEqual(1, len(rows))

    def test_same_instance_id_with_different_employee_is_rejected(self):
        path = Path(self.tmp.name) / "source-instance-conflict.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append([
            "审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门",
            "请假类型", "开始时间", "结束时间", "时长", "审批编号",
            "审批实例ID", "审批流程码", "发起人钉钉UserID",
        ])
        for emp_id, name in (("MT1", "员工甲"), ("MT2", "员工乙")):
            ws.append([
                "完成", "同意", emp_id, name, "总部", "事假",
                "2026-09-01", "2026-09-01", "8小时", "BIZ-1",
                "pi-conflict", "PROC-LEAVE", "ding-1",
            ])
        wb.save(path)
        wb.close()
        with self.assertRaisesRegex(ValueError, "同一审批实例ID"):
            calc_leave.clean_export(str(path))

    def test_pending_approval_and_modified_status_require_review(self):
        for status in ("审批中", "已修改", "未知状态"):
            self.generate([self.row(status=status)])
            self.assertEqual(self.values()[0]["复核状态"], "待确认")

    def test_different_overlapping_approvals_are_not_merged(self):
        self.generate([self.row(approval="A1"), self.row(approval="A2")])
        self.assertEqual(self.values()[1], 2)
        self.edit({"复核状态": "已确认", "确认请假时长": 8, "确认说明": "保留有效申请"})
        self.edit({"复核状态": "已确认", "确认请假时长": 0, "确认说明": "重复申请"}, row=3)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["事假"], 1)

    def test_natural_leave_preserves_days_without_fake_hours(self):
        self.generate([self.row("3天", kind="陪产假", end=datetime(2026, 9, 3, 18, 30))])
        values, _ = self.values()
        self.assertEqual(values["计量口径"], "自然日")
        self.assertIsNone(values["最终请假时长"])
        self.assertEqual(values["最终请假天数"], 3)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["陪产假"], 3)

    def test_marriage_application_days_are_auto_confirmed(self):
        self.generate([self.row(
            "3天", kind="婚假",
            end=datetime(2026, 9, 3, 18, 30),
        )])
        values, issues = self.values()
        self.assertEqual(values["最终请假时长"], 24)
        self.assertEqual(values["最终请假天数"], 3)
        self.assertEqual(values["确认请假时长"], 24)
        self.assertEqual(values["确认请假天数"], 3)
        self.assertEqual(values["复核状态"], "已确认")
        self.assertEqual(values["异常原因"], None)
        self.assertEqual(issues, 0)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["婚假"], 3)

    def test_marriage_application_hours_are_converted_at_eight_hours_per_day(self):
        self.generate([self.row(
            "16小时", kind="婚假",
            end=datetime(2026, 9, 2, 18, 30),
        )])
        values, issues = self.values()
        self.assertEqual(values["最终请假时长"], 16)
        self.assertEqual(values["最终请假天数"], 2)
        self.assertEqual(values["确认请假天数"], 2)
        self.assertEqual(values["复核状态"], "已确认")
        self.assertEqual(issues, 0)

    def test_marriage_cross_month_keeps_only_target_month_days(self):
        self.generate([self.row(
            "5天",
            kind="婚假",
            start=datetime(2026, 8, 30, 9),
            end=datetime(2026, 9, 3, 18, 30),
        )])
        values, issues = self.values()
        self.assertEqual(values["最终请假时长"], 24)
        self.assertEqual(values["最终请假天数"], 3)
        self.assertEqual(issues, 0)
        details = calc_finally.parse_leave_day_details(str(self.path), self.ctx)
        self.assertEqual(set(details["MT0001"]), {date(2026, 9, 1), date(2026, 9, 2), date(2026, 9, 3)})

    def test_marriage_explicit_zero_days_is_confirmed_zero(self):
        self.generate([self.row("0天", kind="婚假")])
        values, issues = self.values()
        self.assertEqual(values["最终请假时长"], 0)
        self.assertEqual(values["最终请假天数"], 0)
        self.assertEqual(values["确认请假天数"], 0)
        self.assertEqual(values["复核状态"], "已确认")
        self.assertEqual(issues, 0)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx), {})

    def test_marriage_missing_application_duration_stays_pending(self):
        self.generate([self.row(None, kind="婚假")])
        values, issues = self.values()
        self.assertEqual(values["复核状态"], "待确认")
        self.assertIn("缺少申请时长依据", values["异常原因"])
        self.assertIsNone(values["最终请假天数"])
        self.assertEqual(issues, 1)

    def test_marriage_requires_completed_and_agreed_approval(self):
        for status, result in (("审批中", "同意"), ("完成", "拒绝")):
            with self.subTest(status=status, result=result):
                source = list(self.row("3天", kind="婚假", status=status))
                source[13] = result
                self.generate([tuple(source)])
                values, issues = self.values()
                self.assertEqual(values["复核状态"], "待确认")
                self.assertIn("审批", values["异常原因"])
                self.assertEqual(issues, 1)

    def test_multiday_correction_requires_conserving_daily_ledger(self):
        self.generate([self.row(12, end=datetime(2026, 9, 2, 18, 30))])
        self.edit({"复核状态": "已确认", "确认请假时长": 12, "确认说明": "首日4小时次日8小时"})
        with self.assertRaisesRegex(ValueError, "确认日分摊"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)
        self.edit({"确认日分摊": "2026-09-01=4;2026-09-02=8"})
        details = calc_finally.parse_leave_day_details(str(self.path), self.ctx)["MT0001"]
        self.assertEqual(details[date(2026, 9, 1)]["事假"], 0.5)
        self.assertEqual(details[date(2026, 9, 2)]["事假"], 1)

    def test_cross_month_does_not_round_system_total_up(self):
        start, end = datetime(2026, 8, 31, 9), datetime(2026, 9, 1, 12)
        allocation, _ = calc_leave.allocate_adjacent_cross_month_system_daily_hours(start, end, 11.2)
        self.assertAlmostEqual(sum(allocation.values()), 11.2)
        self.generate([self.row(11.2, start=start, end=end)])
        self.assertEqual(self.values()[0]["最终请假时长"], 3)

    def test_three_month_allocation_conserves_total(self):
        start, end = datetime(2026, 7, 31, 9), datetime(2026, 9, 1, 18, 30)
        total = sum(8 for offset in range((end.date() - start.date()).days + 1) if (start.date() + timedelta(days=offset)).weekday() < 5)
        allocation, _ = calc_leave.allocate_adjacent_cross_month_system_daily_hours(start, end, total)
        self.assertEqual(sum(allocation.values()), total)
        self.assertEqual({(day.year, day.month) for day in allocation}, {(2026, 7), (2026, 8), (2026, 9)})

    def test_invalid_dates_stay_in_exception_list(self):
        self.generate([self.row(start="坏日期")])
        self.assertEqual(self.values()[1], 1)
        self.generate([self.row(start=datetime(2026, 9, 2), end=datetime(2026, 9, 1))])
        self.assertEqual(self.values()[0]["复核状态"], "待确认")

    def test_wrong_month_and_tampered_ledger_are_rejected(self):
        self.generate([self.row()])
        with self.assertRaisesRegex(ValueError, "月份"):
            calc_finally.parse_leave_summary(str(self.path), schedule_context(2026, 10, set()))
        self.edit({"日分摊": json.dumps({"2026-10-01": {"hours": 8, "days": 1}})})
        with self.assertRaisesRegex(ValueError, "日分摊"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_cross_month_confirmation_uses_target_month_ledger(self):
        self.generate([self.row(None, start=datetime(2026, 8, 31, 9))])
        self.edit({"复核状态": "已确认", "确认请假时长": 8, "确认说明": "核对本月实际8小时"})
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["事假"], 1)
        self.edit({"确认日分摊": "2026-08-31=8;2026-09-01=8"})
        with self.assertRaisesRegex(ValueError, "计算月份"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_confirmation_does_not_hide_out_of_range_daily_allocation(self):
        self.generate([self.row(4)])
        self.edit({"复核状态": "已确认", "确认请假时长": 4, "确认说明": "核对", "确认日分摊": "2026-09-02=4"})
        with self.assertRaisesRegex(ValueError, "超出"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_cross_month_uses_uploaded_month_schedule(self):
        self.ctx["main_working_days"].remove(date(2026, 9, 2))
        self.generate([self.row(32, start=datetime(2026, 8, 31, 9), end=datetime(2026, 9, 3, 18, 30))])
        values, issues = self.values()
        self.assertEqual(values["复核状态"], "自动通过")
        self.assertEqual(values["最终请假时长"], 16)
        self.assertEqual(issues, 0)

    def test_legacy_sheet_missing_review_fields_is_rejected(self):
        wb = openpyxl.Workbook()
        wb.active.append(["工号", "姓名", "请假类型", "最终请假天数"])
        wb.active.append(["MT0001", "员工", "事假", 1])
        wb.save(self.path)
        wb.close()
        with self.assertRaisesRegex(ValueError, "复核状态"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_duplicate_approval_with_different_amounts_is_rejected(self):
        self.generate([self.row(approval="A1"), self.row(approval="A1")])
        self.edit({"复核状态": "已确认", "确认请假时长": 8, "确认说明": "核对有效版本"})
        self.edit({"复核状态": "已确认", "确认请假时长": 4, "确认说明": "冲突版本"}, row=3)
        with self.assertRaisesRegex(ValueError, "同一审批编号"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_confirmed_overlaps_cannot_exceed_one_day(self):
        self.generate([self.row(approval="A1"), self.row(approval="A2")])
        for row in (2, 3):
            self.edit({"复核状态": "已确认", "确认请假时长": 8, "确认说明": "已核对"}, row=row)
        with self.assertRaisesRegex(ValueError, "超过1天"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_fractional_approvals_share_precision_with_day_ledger(self):
        self.generate([
            self.row(3, approval="A1", end=datetime(2026, 9, 1, 12)),
            self.row(3, approval="A2", start=datetime(2026, 9, 1, 14), end=datetime(2026, 9, 1, 17)),
        ])
        summary = calc_finally.parse_leave_summary(str(self.path), self.ctx)
        daily = calc_finally.parse_leave_day_details(str(self.path), self.ctx)
        self.assertEqual(summary["MT0001"]["事假"], 0.75)
        self.assertEqual(daily["MT0001"][date(2026, 9, 1)]["事假"], 0.75)

    def test_maternity_uses_natural_days_even_when_time_is_partial(self):
        self.generate([self.row("1天", kind="产假", end=datetime(2026, 9, 1, 12))])
        summary = calc_finally.parse_leave_summary(str(self.path), self.ctx)
        daily = calc_finally.parse_leave_day_details(str(self.path), self.ctx)
        self.assertEqual(summary["MT0001"]["产假"], 1)
        self.assertEqual(daily["MT0001"][date(2026, 9, 1)]["产假"], 1)

    def test_maternity_checkup_uses_application_hours_without_schedule(self):
        self.generate([
            self.row(
                8,
                kind="产检假",
                emp_id="MT-SD",
                name="单童",
                start=datetime(2026, 9, 9, 13),
                end=datetime(2026, 9, 9, 14),
            )
        ])
        values, issues = self.values()
        self.assertEqual(values["计量口径"], "小时")
        self.assertEqual(values["最终请假时长"], 8)
        self.assertEqual(values["最终请假天数"], 1)
        self.assertEqual(values["复核状态"], "自动通过")
        self.assertEqual(issues, 0)
        summary = calc_finally.parse_leave_summary(str(self.path), self.ctx)
        self.assertEqual(summary["MT-SD"]["产检假"], 1)
        daily = calc_finally.parse_leave_day_details(str(self.path), self.ctx)
        self.assertEqual(daily["MT-SD"][date(2026, 9, 9)]["产检假"], 1)

    def test_maternity_checkup_uses_application_days_directly(self):
        self.generate([self.row("1天", kind="产检假")])
        values, issues = self.values()
        self.assertEqual(values["计量口径"], "自然日")
        self.assertIsNone(values["最终请假时长"])
        self.assertEqual(values["最终请假天数"], 1)
        self.assertEqual(values["复核状态"], "自动通过")
        self.assertEqual(issues, 0)
        self.assertEqual(calc_finally.parse_leave_summary(str(self.path), self.ctx)["MT0001"]["产检假"], 1)

    def test_maternity_checkup_missing_or_unrecognized_duration_stays_pending(self):
        for duration in (None, "8周"):
            with self.subTest(duration=duration):
                self.generate([self.row(duration, kind="产检假")])
                values, issues = self.values()
                self.assertEqual(values["复核状态"], "待确认")
                self.assertEqual(issues, 1)
                self.assertIsNone(values["最终请假天数"])

    def test_maternity_checkup_incomplete_or_rejected_approval_stays_pending(self):
        pending = list(self.row(8, kind="产检假", status="审批中"))
        rejected = list(self.row(8, kind="产检假"))
        rejected[13] = "拒绝"
        for source_row in (pending, rejected):
            self.generate([tuple(source_row)])
            values, issues = self.values()
            self.assertEqual(values["复核状态"], "待确认")
            self.assertEqual(issues, 1)

    def test_maternity_checkup_only_keeps_target_month_intersections(self):
        self.generate([
            self.row(
                8,
                kind="产检假",
                start=datetime(2026, 8, 31, 9),
                end=datetime(2026, 8, 31, 18),
            )
        ])
        workbook = openpyxl.load_workbook(self.path, data_only=True)
        try:
            self.assertEqual(workbook["请假明细"].max_row, 1)
        finally:
            workbook.close()

    def test_unrecognized_workbook_cannot_be_treated_as_no_leave(self):
        wb = openpyxl.Workbook()
        wb.active.append(["无效表头"])
        wb.active.append(["无效数据"])
        wb.save(self.path)
        wb.close()
        with self.assertRaisesRegex(ValueError, "未找到"):
            calc_finally.parse_leave_summary(str(self.path), self.ctx)

    def test_modified_version_without_completion_time_cannot_revive_old_approval(self):
        path = Path(self.tmp.name) / "source.xlsx"
        wb = openpyxl.Workbook()
        ws = wb.active
        ws.append(["审批状态", "审批结果", "发起人工号", "发起人姓名", "发起人部门", "请假类型", "开始时间", "结束时间", "时长", "审批编号", "完成时间"])
        for status, finish in (("完成", "2026-09-02 12:00"), ("已修改", None)):
            ws.append([status, "同意", "MT1", "员工", "总部", "事假", "2026-09-01", "2026-09-01", "8小时", "A1", finish])
        wb.save(path)
        wb.close()
        with contextlib.redirect_stdout(io.StringIO()):
            rows = calc_leave.clean_export(str(path))
            calc_leave.process(rows, str(self.path), self.ctx)
        self.assertEqual(self.values()[0]["复核状态"], "待确认")
        self.assertIn("版本待确认", self.values()[0]["异常原因"])


if __name__ == "__main__":
    unittest.main()
