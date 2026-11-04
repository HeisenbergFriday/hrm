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


if __name__ == '__main__':
    unittest.main()
