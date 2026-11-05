import json
import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import Mock

import openpyxl

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT))
import runner
import integrity


class IntegrityTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)

    def workbook(self, filename, rows):
        path = self.root / filename
        wb = openpyxl.Workbook()
        for row in rows:
            wb.active.append(row)
        wb.save(path)
        wb.close()
        return str(path)

    def test_missing_month_never_calculates_and_exports_audit(self):
        calculate = Mock()
        outputs = integrity.run('parttime', {}, self.root, calculate)
        calculate.assert_not_called()
        wb = openpyxl.load_workbook(outputs[0]['path'])
        self.assertEqual(set(wb.sheetnames), {'文件检查', '数据检查'})
        wb.close()
        meta = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual(meta['status'], 'needs_review')

    def test_filename_header_month_conflict(self):
        path = self.workbook('9月考勤.xlsx', [['2026年10月'], ['姓名', '工号', 1, 2, 3], ['张三', 'MT001', 1, 1, 1]])
        report = integrity.preflight('parttime', {'parttime_target_month': '2026-09', 'parttime_attendance_detail': [path]})
        self.assertTrue(any(item['kind'] == '月份冲突' for item in report['issues']))

    def test_duplicate_number_blank_name_and_empty_month(self):
        path = self.workbook('2026年9月.xlsx', [['姓名', '工号', 1, 2, 3], ['刘芳', 'MT001', None, None, None], ['刘芳', 'MT001', 1, 1, 1], [None, 'MT002', 1, 1, 1]])
        report = integrity.preflight('parttime', {'parttime_target_month': '2026-09', 'parttime_attendance_detail': [path]})
        self.assertTrue({'重复工号', '空姓名', '整月空白'}.issubset({item['kind'] for item in report['issues']}))

    def test_real_zhangsan_with_different_numbers_stays_two_people(self):
        path = self.workbook('2026年9月.xlsx', [['姓名', '工号', '职位', 1, 2, 3], ['张三', 'MT001', '兼职', 1, 1, 1], ['张三', 'MT009', '兼职', 1, 1, 1]])
        report = integrity.preflight('parttime', {'parttime_target_month': '2026-09', 'parttime_attendance_detail': [path]})
        self.assertEqual(report['status'], 'passed', report)
        self.assertEqual(len(runner.part.parse_attendance_detail(path)), 2)

    def test_ambiguous_name_without_number_blocks(self):
        path = self.workbook('2026年9月.xlsx', [['姓名', '工号', 1, 2, 3], ['张三', 'MT001', 1, 1, 1], ['张三', 'MT009', 1, 1, 1], ['张三', None, 1, 1, 1]])
        report = integrity.preflight('parttime', {'parttime_target_month': '2026-09', 'parttime_attendance_detail': [path]})
        self.assertTrue(any(item['kind'] == '同名待确认' for item in report['issues']))

    def test_roster_missing_person_is_not_hidden_by_output_count(self):
        schedule = self.root / '2026年6月作息.xlsx'
        schedule.write_bytes(runner.templates.build_schedule_template())
        detail = self.workbook('2026年6月考勤.xlsx', [['姓名', '工号', '职位', 1, 2, 3], ['刘芳', 'JZ001', '兼职', 1, 1, 1]])
        roster = self.root / 'roster.json'
        roster.write_text(json.dumps({'employees': [{'emp_no': 'JZ001', 'name': '刘芳', 'emp_type': '兼职'}, {'emp_no': 'JZ002', 'name': '王芳', 'emp_type': '实习'}]}), encoding='utf-8')
        result = runner.part.generate_parttime_summary(str(self.root / 'result.xlsx'), attendance_detail_paths=[detail], default_schedule_path=str(schedule), target_month='2026-06', expected_roster_path=str(roster), special_default_names=[])
        self.assertEqual(result['audit']['expected_people'], 2)
        self.assertEqual(result['audit']['recognized_people'], 1)
        self.assertEqual(result['audit']['missing_people'], 1)
        self.assertEqual(result['audit']['status'], 'needs_review')
        self.assertEqual(result['rows'], 2)
        wb = openpyxl.load_workbook(self.root / 'result.xlsx', data_only=True)
        ws = wb['6月兼职出勤汇总']
        self.assertIsInstance(ws.cell(3, 34).value, (int, float))
        wb.close()

    def test_template_example_is_explicit(self):
        path = self.root / 'template.xlsx'
        path.write_bytes(runner.templates.build_roster_template())
        wb = openpyxl.load_workbook(path)
        self.assertEqual(wb.active.cell(2, 1).value, '【示例】张三')
        wb.close()

    def test_cross_month_approval_is_allowed(self):
        path = self.workbook('跨月请假.xlsx', [['姓名', '工号', '开始时间', '结束时间'], ['张三', 'MT001', '2026-08-31', '2026-09-02']])
        report = integrity.preflight('leave', {'leave_target_month': '2026-09', 'leave_src': path})
        self.assertEqual(report['status'], 'passed')

    def parttime_config(self, people=None, rows=None):
        schedule = self.root / '2026年6月作息.xlsx'
        schedule.write_bytes(runner.templates.build_schedule_template())
        detail = self.workbook('2026年6月考勤.xlsx', [
            ['姓名', '工号', '职位', 1, 2, 3], *(rows or [['张三', 'MT001', '兼职', 1, 1, 1]])])
        roster = self.root / 'roster.json'
        roster.write_text(json.dumps({'employees': people or [
            {'emp_no': 'MT001', 'name': '张三', 'emp_type': '兼职'}]}), encoding='utf-8')
        return {'parttime_target_month': '2026-06', 'parttime_attendance_detail': [detail],
                'parttime_default_schedule': str(schedule), 'parttime_roster': str(roster), 'part_special_names': ''}

    def test_missing_people_blocks_before_calculator_and_has_counts(self):
        config = self.parttime_config(people=[{'emp_no': f'MT{i:03}', 'name': '张三' if i == 1 else f'员工{i}', 'emp_type': '兼职'} for i in range(1, 64)])
        calculate = Mock()
        outputs = integrity.run('parttime', config, self.root, calculate)
        calculate.assert_not_called()
        report = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual((report['expected_people'], report['recognized_people'], report['missing_people']), (63, 1, 62))
        self.assertEqual(outputs[0]['kind'], 'audit')
        wb = openpyxl.load_workbook(outputs[0]['path'], data_only=True)
        self.assertIn('人员核对', [row[0].value for row in wb['数据检查']])
        wb.close()

    def test_unmatched_schedule_blocks(self):
        config = self.parttime_config()
        config['parttime_schedules'] = [self.workbook('2026年6月排班.xlsx', [['姓名', '工号', 1, 2, 3], ['李四', 'MT002', 1, 0, 1]])]
        report = integrity.preflight('parttime', config)
        self.assertIn('排班未匹配', {item['kind'] for item in report['issues']})

    def test_conflict_records_both_filenames_and_zero_then_explicit_priority(self):
        config = self.parttime_config()
        monthly = self.workbook('2026年6月月度汇总.xlsx', [
            ['姓名', '工号', '职位', '考勤结果', None, None],
            [None, None, None, 1, 2, 3], ['张三', 'MT001', '兼职', 0, 1, 1]])
        config['parttime_monthly'] = [monthly]
        calculate = Mock()
        integrity.run('parttime', config, self.root, calculate)
        calculate.assert_not_called()
        config['parttime_conflict_policy'] = 'attendance_first'
        report = integrity.preflight('parttime', config)
        self.assertEqual(report['status'], 'passed', report)
        self.assertEqual(report['resolved_conflict_count'], 1)
        self.assertEqual({v['source'] for v in report['conflicts'][0]['values']}, {'2026年6月考勤.xlsx', '2026年6月月度汇总.xlsx'})
        self.assertEqual({v['value'] for v in report['conflicts'][0]['values']}, {0, 1})
        outputs = integrity.run('parttime', config, self.root, runner.run_parttime)
        wb = openpyxl.load_workbook(outputs[0]['path'], data_only=True)
        self.assertEqual(wb['6月兼职出勤汇总'].cell(3, 4).value, 1)
        self.assertEqual(wb['6月兼职出勤汇总'].cell(3, 34).value, 3)
        rows = list(wb['数据检查'].values)
        self.assertTrue(any(row[0] == '来源冲突' and row[2] == '已处理' and '2026年6月考勤.xlsx' in row[3] for row in rows))
        self.assertTrue(any(row[0] == '检查状态' and row[3] == '计算完成' for row in rows))
        wb.close()

    def test_equal_priority_conflict_remains_blocked(self):
        config = self.parttime_config()
        config['parttime_attendance_detail'].append(self.workbook('2026年6月另一考勤.xlsx', [
            ['姓名', '工号', '职位', 1, 2, 3], ['张三', 'MT001', '兼职', 0, 1, 1]]))
        config['parttime_conflict_policy'] = 'attendance_first'
        report = integrity.preflight('parttime', config)
        self.assertEqual(report['status'], 'needs_review')
        self.assertEqual(report['conflict_count'], 1)
        self.assertTrue(all(v['source'] != '已合并文件' for v in report['conflicts'][0]['values']))

    def test_same_names_with_ids_export_two_rows_and_no_pending_audit(self):
        config = self.parttime_config(
            people=[{'emp_no': code, 'name': '张三', 'emp_type': '兼职'} for code in ['MT001', 'MT009']],
            rows=[['张三', code, '兼职', 1, 1, 1] for code in ['MT001', 'MT009']])
        outputs = integrity.run('parttime', config, self.root, runner.run_parttime)
        report = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual(report['status'], 'passed', report)
        self.assertEqual(report['recognized_people'], 2)
        wb = openpyxl.load_workbook(outputs[0]['path'], data_only=True)
        self.assertEqual({wb['6月兼职出勤汇总'].cell(row, 1).value for row in (3, 4)}, {'MT001', 'MT009'})
        self.assertFalse(any(row[2] in {'待确认', '待核对', '待处理'} for row in list(wb['数据检查'].values)[1:]))
        wb.close()

    def test_duplicate_nameless_roster_cannot_be_collapsed(self):
        config = self.parttime_config(people=[{'name': '张三', 'emp_type': '兼职'}] * 2)
        report = integrity.preflight('parttime', config)
        self.assertEqual(report['status'], 'needs_review')
        self.assertIn('同名员工', str(report['issues']))

    def test_full_month_real_fixed_employee_has_no_fill_warning(self):
        config = self.parttime_config()
        config['part_special_names'] = '张三'
        workdays = runner.part._build_default_day_maps(2026, 6, config['parttime_default_schedule'])['main']
        config['parttime_attendance_detail'] = [self.workbook('2026年6月完整考勤.xlsx', [
            ['姓名', '工号', '职位', *range(1, 31)],
            ['张三', 'MT001', '兼职', *[1 if day in workdays else 0 for day in range(1, 31)]]])]
        outputs = integrity.run('parttime', config, self.root, runner.run_parttime)
        report = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual(report['status'], 'passed', report)
        self.assertEqual(report['fallback_count'], 0)

    def test_fill_days_are_yellow_commented_and_pending(self):
        config = self.parttime_config()
        config['part_special_names'] = '张三'
        outputs = integrity.run('parttime', config, self.root, runner.run_parttime)
        report = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual(report['status'], 'needs_review')
        self.assertGreater(report['fallback_count'], 0)
        wb = openpyxl.load_workbook(outputs[0]['path'])
        ws = wb['6月兼职出勤汇总']
        self.assertEqual(ws.cell(3, 34).value, ws.cell(3, 35).value + ws.cell(3, 36).value)
        self.assertIn('系统按工作日补填', ws.cell(3, 7).comment.text)
        self.assertEqual(ws.cell(3, 7).fill.fgColor.rgb, runner.part.ATTENDANCE_ALERT_FILL.fgColor.rgb)
        self.assertTrue(any(row[0] == '检查状态' and row[3] == '计算未完成' for row in wb['数据检查'].values))
        wb.close()

    def test_formula_only_total_blocks_export(self):
        path = self.workbook('结果.xlsx', [['姓名', '出勤天数'], ['张三', '=1+1']])
        report = {'status': 'passed', 'files': [], 'issues': [], 'target_month': '2026-06'}
        outputs = integrity.finish([{'path': path, 'kind': 'export'}], report, self.root)
        self.assertEqual(outputs[0]['kind'], 'audit')
        self.assertEqual(report['status'], 'needs_review')

    def test_calendar_without_dates_and_approval_missing_dates_fail(self):
        path = self.workbook('2026年6月作息.xlsx', [['2026年6月'], ['随意文字']])
        report = integrity.preflight('leave', {'leave_target_month': '2026-06', 'leave_schedule': path})
        self.assertEqual(report['status'], 'needs_review')
        path = self.workbook('请假.xlsx', [['姓名', '工号', '假期'], ['张三', 'MT001', '事假']])
        report = integrity.preflight('leave', {'leave_target_month': '2026-06', 'leave_src': path})
        self.assertIn('缺少日期', {item['kind'] for item in report['issues']})

    def test_upload_audit_checks_original_filename_and_header(self):
        path = self.workbook('temporary.xlsx', [['2026年10月'], ['姓名', '工号', 1], ['张三', 'MT001', 1]])
        outputs = runner.action_audit({'audit_module': 'parttime', 'target_month': '2026-09', 'files': [
            {'name': '9月考勤.xlsx', 'path': path, 'field': 'parttime_attendance_detail'}]}, self.root)
        report = json.loads(Path(outputs[0]['path']).read_text(encoding='utf-8'))
        self.assertFalse(report['ok'])
        self.assertEqual(report['files'][0]['file_name'], '9月考勤.xlsx')
        self.assertIn('月份冲突', str(report['meta']))

    def test_roster_without_number_matches_unique_source_once(self):
        config = self.parttime_config(people=[{'name': '张三', 'emp_type': '兼职'}])
        outputs = integrity.run('parttime', config, self.root, runner.run_parttime)
        report = json.loads(Path(outputs[-1]['path']).read_text(encoding='utf-8'))
        self.assertEqual(report['status'], 'passed', report)
        wb = openpyxl.load_workbook(outputs[0]['path'], data_only=True)
        self.assertEqual(wb['6月兼职出勤汇总'].max_row, 3)
        self.assertEqual(wb['6月兼职出勤汇总'].cell(3, 1).value, 'MT001')
        wb.close()

    def test_source_person_outside_roster_and_unreadable_values_are_blocked(self):
        config = self.parttime_config(rows=[['张三', 'MT001', '兼职', 1, 1, 1], ['李四', 'MT009', '兼职', 1, 1, 1]])
        report = integrity.preflight('parttime', config)
        self.assertIn('花名册未匹配', {item['kind'] for item in report['issues']})
        config = self.parttime_config(rows=[['张三', 'MT001', '兼职', '未知状态', '未知状态', '未知状态']])
        report = integrity.preflight('parttime', config)
        self.assertIn('整月空白', {item['kind'] for item in report['issues']})

    def test_schedule_template_colors_match_calculator(self):
        config = self.parttime_config()
        context = runner.part.calc_leave.load_schedule_context(config['parttime_default_schedule'], data_only=True)
        self.assertEqual(len(context['main_working_days']), 21)
        self.assertGreater(len(context['main_expected_attendance_days']), 21)


if __name__ == '__main__':
    unittest.main()
