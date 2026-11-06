from datetime import date, datetime
from pathlib import Path
import sys
import tempfile
import unittest


HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))

from overtime import fill_overtime_fields as ot
from overtime.rules_engine import get_default_config
from rules_adapter import config_from_dict


class OvertimeRuleUpdateTests(unittest.TestCase):
    def test_roster_suffix_keeps_approval_parent_but_real_transfer_wins(self):
        ws = self._workbook([
            'MT0156', '潘烈宇', '运营管理中心', '运营支撑部', '成都运营-成都客服-系统客服',
            datetime(2026, 10, 4, 9), datetime(2026, 10, 4, 18),
            date(2026, 10, 4), None, 8, 8, None, None,
        ])
        header = ot.build_header_map(ws, 1)
        for roster, expected in (
            ({'dept1': '成都运营', 'dept2': '成都客服', 'dept3': '系统客服'},
             ('运营管理中心', '运营支撑部', '成都运营-成都客服-系统客服', '成都组')),
            ({'dept1': '研发中心', 'dept2': '平台部', 'dept3': '开发组'},
             ('研发中心', '平台部', '开发组', '成都组')),
        ):
            with self.subTest(roster=roster):
                actual = ot._resolve_employee_department_fields(
                    ws, 2, header, 'MT0156', '潘烈宇', {'潘烈宇': '成都组'}, {'MT0156': roster})
                self.assertEqual(actual, expected)
        ws.parent.close()

    def test_ops_mixed_daily_details_share_total_cap_in_complete_output(self):
        import openpyxl

        for total, details, expected in ((16, (8, None, 8), (8, 0, 8)),
                                         (8, (None, None, 8), (0, 0, 8)),
                                         (10, (8, 8, 8), (8, 2, 0))):
            with self.subTest(total=total, details=details), tempfile.TemporaryDirectory() as tmp:
                rows = [ot.CLEAN_OUTPUT_COLS] + [(
                    'MT0156', '潘烈宇', '运营管理中心', '运营支撑部', '成都客服',
                    datetime(2026, 9, 25, 9), datetime(2026, 9, 27, 18),
                    date(2026, 9, day), None, hours, total, None, None,
                ) for day, hours in zip((25, 26, 27), details)]
                ot._reset_attendance_runtime_state()
                ot._reset_work_calendar_runtime_state()
                target = Path(tmp) / 'result.xlsx'
                ot.process_overtime(rows, str(target), {}, {}, target_month_anchor=date(2026, 9, 1))
                wb = openpyxl.load_workbook(target, data_only=True)
                try:
                    data = list(wb.active.values)
                    records = [dict(zip(data[0], r)) for r in data[1:]]
                    self.assertEqual(tuple(r['最终加班时长（小时）'] for r in records), expected)
                    self.assertEqual(sum(r['最终加班时长（小时）'] for r in records), total)
                    self.assertTrue(all(r['加班类型'] == '加班工资' for r in records))
                finally:
                    wb.close()

    def test_runner_uploads_use_target_month_and_keep_roster_parent(self):
        import openpyxl
        import runner

        # Real holiday data in three periods, no production date overrides.
        for year, month, days, legal_day in ((2026, 9, (25, 26, 27), 25),
                                             (2026, 10, (1, 4, 5), 1),
                                             (2025, 10, (1, 4, 5), 1)):
            with self.subTest(year=year, month=month), tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                approval = root / 'approval.xlsx'
                roster = root / 'roster.xlsx'
                schedule1, schedule2 = root / 'schedule1.xlsx', root / 'schedule2.xlsx'
                wb = openpyxl.Workbook()
                ws = wb.active
                ws.append(['审批状态', '审批结果', '审批编号', '发起人工号', '发起人姓名',
                           '发起人部门', '开始时间', '结束时间', '明细', '加班时长', '时长'])
                for day in days:
                    ws.append(['完成', '同意', 'A1', 'MT0156', '潘烈宇',
                               '运营管理中心-运营支撑部-成都运营-成都客服-系统客服',
                               datetime(year, month, days[0], 9), datetime(year, month, days[-1], 18),
                               date(year, month, day), 8, 24])
                # Non-target-year records must not affect validation/calculation.
                ws.append(['完成', '同意', 'OLD', 'MT-X', '历史员工', '总部-研发',
                           datetime(2000, 1, 1, 9), datetime(2000, 1, 1, 18), date(2000, 1, 1), 8, 8])
                wb.save(approval)
                wb.close()
                wb = openpyxl.Workbook()
                wb.active.append(['工号', '姓名', '一级部门', '二级部门', '三级部门'])
                wb.active.append(['MT0156', '潘烈宇', '成都运营', '成都客服', '系统客服'])
                wb.save(roster)
                wb.close()
                for path, label, only_name in ((schedule1, '客服', '甲'), (schedule2, '运营', '乙')):
                    wb = openpyxl.Workbook()
                    ws = wb.active
                    ws.title = f'{year}{month:02}' if label == '客服' else '历史和本月'
                    # Two week blocks and a duplicate sheet exercise generic month filtering.
                    for block_days, block_year in ((days, year - 1), (days, year)):
                        ws.append(['日期', *[date(block_year, month, d) for d in block_days]])
                        ws.append([None, '星期', '星期', '星期'])
                        ws.append(['潘烈宇', '09:00-18:00', '09:00-18:00', '09:00-18:00'])
                        ws.append([f'缺申请{only_name}', '09:00-18:00', 'OFF', 'OFF'])
                        # Hidden characters should not produce a second person.
                        ws.append(['唐\u200b华', '09:00-18:00', 'OFF', 'OFF'])
                    clone = wb.copy_worksheet(ws)
                    clone.title = ws.title + '-副本'
                    wb.save(path)
                    wb.close()
                output = root / 'output'
                output.mkdir()
                runner.run_overtime({'overtime_src': str(approval), 'overtime_roster': str(roster),
                                     'overtime_schedules': [str(schedule1), str(schedule2)],
                                     'overtime_target_month': f'{year}-{month:02}'}, output)
                wb = openpyxl.load_workbook(output / '加班明细_回填.xlsx', data_only=True)
                try:
                    data = list(wb.active.values)
                    records = [dict(zip(data[0], r)) for r in data[1:]]
                    pan = [r for r in records if r['发起人姓名'] == '潘烈宇']
                    self.assertEqual(len(pan), 3)
                    for r, day in zip(pan, days):
                        self.assertEqual(r['加班类型'], '加班工资')
                        self.assertEqual(r['最终加班时长（小时）'], 8)
                        self.assertEqual(r['3倍加班小时'] or 0, 8 if day == legal_day else 0)
                        self.assertEqual(r['2倍加班小时'] or 0, 0 if day == legal_day else 8)
                    missing = [r for r in records if r['系统操作'] == '疑似漏报']
                    self.assertEqual(len(missing), 3)
                    self.assertEqual({r['发起人姓名'] for r in missing}, {'缺申请甲', '缺申请乙', '唐华'})
                    self.assertTrue(all(r['最终加班时长（小时）'] is None for r in missing))
                    self.assertEqual(len(records), 6)
                finally:
                    wb.close()

    def test_ops_holiday_pay_is_builtin_despite_custom_rule_omissions(self):
        from rules_engine import classify_employee, determine_action, LEGAL_HOLIDAY, HOLIDAY_ADJUST_REST, ORDINARY_WEEKEND

        config = self._custom_rules_without_ops()
        for departments in (
            ("运营支撑部", "", ""),
            ("运营管理中心", "运营支撑部", "成都运营-成都客服-系统客服"),
            ("总部", "运营管理中心", "运营支撑部-客服"),
        ):
            with self.subTest(departments=departments):
                groups = classify_employee(config, *departments, "")
                self.assertIn("运营支撑部", groups)
                self.assertEqual(determine_action(config, LEGAL_HOLIDAY, groups), ("加班工资", 3.0))
                self.assertEqual(determine_action(config, HOLIDAY_ADJUST_REST, groups), ("加班工资", 2.0))
                self.assertEqual(determine_action(config, ORDINARY_WEEKEND, groups), ("调休", 1.0))
        other = classify_employee(config, "总部", "研发部", "", "")
        self.assertEqual(determine_action(config, HOLIDAY_ADJUST_REST, other), ("调休", 1.0))

    def test_ops_mid_autumn_full_output_uses_builtin_pay(self):
        import openpyxl

        config = self._custom_rules_without_ops()
        rows = [ot.CLEAN_OUTPUT_COLS]
        for day in (25, 26, 27):
            rows.append((
                "MT0156", "潘烈宇", "运营管理中心", "运营支撑部", "成都运营-成都客服-系统客服",
                datetime(2026, 9, 25, 12), datetime(2026, 9, 27, 22),
                date(2026, 9, day), date(2026, 9, day), 8, 24, None, None,
            ))
        ot._reset_attendance_runtime_state()
        ot._reset_work_calendar_runtime_state()
        with tempfile.TemporaryDirectory() as temp:
            target = Path(temp) / "result.xlsx"
            ot.process_overtime(rows, str(target), {}, {}, rules_config=config, target_month_anchor=date(2026, 9, 1))
            wb = openpyxl.load_workbook(target, data_only=True)
            try:
                data = list(wb.active.values)
                records = [dict(zip(data[0], row)) for row in data[1:]]
                self.assertEqual(len(records), 3)
                for record, day in zip(records, (25, 26, 27)):
                    self.assertEqual(record["加班类型"], "加班工资")
                    self.assertEqual(record["最终加班时长（小时）"], 8)
                    self.assertEqual(record["2倍加班小时"] or 0, 0 if day == 25 else 8)
                    self.assertEqual(record["3倍加班小时"] or 0, 8 if day == 25 else 0)
            finally:
                wb.close()

    def _custom_rules_without_ops(self):
        # Legacy/custom settings can omit the department mapping and put a
        # generic comp-time rule before the built-in department pay rule.
        return config_from_dict({
            "department_rules": [{"group_name": "研发", "match_field": "二级部门", "match_value": "研发部"}],
            "premium_rules": [
                {"priority": i, "date_type": kind, "department_group": "全部", "action": "调休", "multiplier": 1}
                for i, kind in enumerate(("LEGAL_HOLIDAY", "HOLIDAY_ADJUST_REST", "ORDINARY_WEEKEND", "NORMAL_WORKDAY"))
            ],
            "params": {"schedule_augment_rest_dept_group": "研发"},
        })

    def test_cross_day_total_context_keeps_builtin_ops_holiday_pay(self):
        ot._reset_attendance_runtime_state()
        ws = self._workbook(*[
            ["MT0156", "潘烈宇", "运营管理中心", "运营支撑部", "成都客服",
             datetime(2026, 9, 25, 12), datetime(2026, 9, 27, 22),
             date(2026, 9, day), date(2026, 9, day), None, 24, None, None]
            for day in (25, 26, 27)
        ])
        header = ot.ensure_target_columns(ws, 1, ot.build_header_map(ws, 1))
        config = self._custom_rules_without_ops()
        contexts = ot._build_general_overtime_contexts(ws, header, [2, 3, 4], None, config)
        self.assertEqual(set(contexts), {2, 3, 4})
        self.assertEqual(sum(x["final_hours"] for x in contexts.values()), 24)
        for row, day in zip((2, 3, 4), (25, 26, 27)):
            ot.fill_row(ws, row, header, {}, date(2026, 9, 1), rules_config=config,
                        general_overtime_contexts=contexts)
            self.assertEqual(ws.cell(row, header["加班类型"]).value, "加班工资")
            self.assertEqual(ws.cell(row, header["最终加班时长（小时）"]).value, 8)
            self.assertEqual(ws.cell(row, header["2倍加班小时"]).value or 0, 0 if day == 25 else 8)
            self.assertEqual(ws.cell(row, header["3倍加班小时"]).value or 0, 8 if day == 25 else 0)

    def test_builtin_ops_pay_keeps_existing_personal_rest_exception(self):
        ot._reset_attendance_runtime_state()
        for use_context in (False, True):
            with self.subTest(use_context=use_context):
                ws = self._workbook([
                    "EXCEPTION", "陈星雨", "运营管理中心", "运营支撑部", "成都客服",
                    datetime(2026, 9, 26, 9), datetime(2026, 9, 26, 18),
                    date(2026, 9, 26), date(2026, 9, 26), 8, 8, None, None,
                ])
                header = ot.ensure_target_columns(ws, 1, ot.build_header_map(ws, 1))
                config = self._custom_rules_without_ops()
                contexts = ot._build_general_overtime_contexts(ws, header, [2], None, config) if use_context else None
                ot.fill_row(ws, 2, header, {}, date(2026, 9, 1), rules_config=config,
                            general_overtime_contexts=contexts)
                self.assertEqual(ws.cell(2, header["加班类型"]).value, "调休")
                self.assertEqual(ws.cell(2, header["最终加班时长（小时）"]).value, 8)

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
