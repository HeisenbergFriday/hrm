import json
import sys
import tempfile
import unittest
from datetime import date
from pathlib import Path

ROOT = Path(__file__).resolve().parent
sys.path.insert(0, str(ROOT / "finally"))
import calc_finally as fin


class AutomaticResignationWindowTests(unittest.TestCase):
    def test_keeps_previous_current_following_month_only(self):
        payload = {
            "employees": [
                {"emp_no": "MT-JUL", "name": "上月", "resign_date": "2026-07-31"},
                {"emp_no": "MT-AUG", "name": "当月", "resign_date": "2026-08-15"},
                {"emp_no": "MT-SEP", "name": "下月", "resign_date": "2026-09-01"},
                {"emp_no": "MT-JUN", "name": "过早", "resign_date": "2026-06-30"},
                {"emp_no": "MT-NODATE", "name": "无日期"},
            ]
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "auto.json"
            path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
            rows = fin.parse_auto_resign_json(str(path), {"year": 2026, "month": 8})
        self.assertEqual([row["emp_no"] for row in rows], ["MT-JUL", "MT-AUG", "MT-SEP"])

    def test_year_boundary_window(self):
        payload = {"employees": [
            {"emp_no": "MT-DEC", "name": "去年十二月", "resign_date": "2025-12-31"},
            {"emp_no": "MT-JAN", "name": "今年一月", "resign_date": "2026-01-02"},
            {"emp_no": "MT-FEB", "name": "今年二月", "resign_date": "2026-02-01"},
            {"emp_no": "MT-NOV", "name": "去年十一月", "resign_date": "2025-11-30"},
        ]}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "auto.json"
            path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
            rows = fin.parse_auto_resign_json(str(path), {"year": 2026, "month": 1})
        self.assertEqual({row["emp_no"] for row in rows}, {"MT-DEC", "MT-JAN", "MT-FEB"})

    def test_does_not_turn_numeric_user_id_into_employee_number(self):
        payload = {"employees": [{"user_id": "123456", "name": "张三", "resign_date": "2026-08-01"}]}
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "auto.json"
            path.write_text(json.dumps(payload, ensure_ascii=False), encoding="utf-8")
            rows = fin.parse_auto_resign_json(str(path), {"year": 2026, "month": 8})
        self.assertEqual(rows[0]["emp_no"], "")

    def test_monthly_attendance_adds_person_missing_from_roster(self):
        roster = [{"emp_no": "MT0001", "name": "在职员工"}]
        attendance = [{"emp_no": "MT0639", "name": "漏在人事花名册的人"}]
        merged, stats = fin.merge_employee_sources(
            roster,
            attendance,
            mark_source=False,
            source_flag="source_target_month_activity",
        )
        self.assertEqual({row["emp_no"] for row in merged}, {"MT0001", "MT0639"})
        self.assertEqual(stats["added_count"], 1)
        self.assertNotIn("source_attendance_roster", merged[-1])
        self.assertTrue(merged[-1]["source_target_month_activity"])

    def test_monthly_attendance_keeps_existing_intern_in_final_table(self):
        roster = [{"emp_no": "MT0001", "name": "实习员工", "emp_type": "实习生"}]
        attendance = [{"emp_no": "MT0001", "name": "实习员工"}]
        merged, stats = fin.merge_employee_sources(
            roster,
            attendance,
            mark_source=False,
            source_flag="source_target_month_activity",
        )
        self.assertEqual(stats["matched_count"], 1)
        self.assertTrue(merged[0]["source_target_month_activity"])
        self.assertFalse(fin._is_final_table_excluded_employee(merged[0]))

    def test_monthly_attendance_keeps_person_but_drops_numeric_user_id(self):
        attendance = [{"emp_no": "17772545162726343", "name": "数字账号员工"}]
        merged, stats = fin.merge_employee_sources([], attendance, mark_source=False)
        self.assertEqual(stats["added_count"], 1)
        self.assertEqual(merged[0]["name"], "数字账号员工")
        self.assertEqual(merged[0]["emp_no"], "")

    def test_activity_keys_keep_business_number_and_name(self):
        rows, unresolved = fin.employees_from_activity_keys(
            ["MT0121", "刘明湖", "17772545162726343"]
        )
        self.assertEqual(rows, [
            {"emp_no": "MT0121", "name": ""},
            {"emp_no": "", "name": "刘明湖"},
        ])
        self.assertEqual(unresolved, 1)

    def test_activity_records_keep_name_for_business_number(self):
        rows, unresolved = fin.employees_from_activity_records([
            {"emp_no": "MT0121", "name": "刘明湖"},
            {"emp_no": "MT0121", "name": "刘明湖"},
            {"emp_no": "17772545162726343", "name": "数字账号员工"},
        ])
        self.assertEqual(rows, [
            {"emp_no": "MT0121", "name": "刘明湖"},
            {"emp_no": "", "name": "数字账号员工"},
        ])
        self.assertEqual(unresolved, 0)

    def test_deduplicate_employee_sources_keeps_richest_formal_identity(self):
        employees, stats = fin.deduplicate_employee_sources([
            {"emp_no": "MT0440", "name": "王敏", "hire_date": date(2025, 4, 24)},
            {"emp_no": "MT0440", "name": "王敏"},
        ])
        self.assertEqual(len(employees), 1)
        self.assertEqual(employees[0]["hire_date"], date(2025, 4, 24))
        self.assertEqual(stats["duplicate_count"], 1)

    def test_mt_business_number_is_preferred_over_wb_for_same_employee(self):
        employees = [{"emp_no": "WB0003", "name": "庄雨倩"}]
        enriched = fin.apply_attendance_identity(
            employees,
            [{"emp_no": "MT0684", "name": "庄雨倩"}],
        )
        self.assertEqual(enriched[0]["emp_no"], "MT0684")


if __name__ == "__main__":
    unittest.main()
