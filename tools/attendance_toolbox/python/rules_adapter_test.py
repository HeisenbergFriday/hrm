# -*- coding: utf-8 -*-
"""Unit tests for rules_adapter (custom rules resolution)."""
from __future__ import annotations

import json
import sys
import tempfile
import unittest
from pathlib import Path

ROOT = Path(__file__).resolve().parent
for p in (ROOT, ROOT / "overtime"):
    s = str(p)
    if s not in sys.path:
        sys.path.insert(0, s)

from rules_adapter import config_from_dict, config_to_preview_dict, resolve_overtime_config
from rules_engine import get_default_config, save_config


class RulesAdapterTests(unittest.TestCase):
    def test_default_resolve(self) -> None:
        cfg, meta = resolve_overtime_config({})
        self.assertEqual(meta["source"], "default")
        self.assertGreater(len(cfg.premium_rules), 0)
        self.assertGreater(len(cfg.department_rules), 0)

    def test_custom_json_changes_department_rules(self) -> None:
        payload = {
            "premium_rules": [
                {
                    "priority": 1,
                    "date_type": "LEGAL_HOLIDAY",
                    "department_group": "全部",
                    "action": "加班工资",
                    "multiplier": 3.0,
                }
            ],
            "department_rules": [
                {
                    "group_name": "测试部门组",
                    "match_field": "一级部门",
                    "match_method": "包含",
                    "match_value": "测试部",
                }
            ],
            "params": {"standard_hours_per_day": 7.5, "no_punch_mark": "未打卡"},
        }
        cfg, meta = resolve_overtime_config({"rules_json": json.dumps(payload, ensure_ascii=False)})
        self.assertEqual(meta["source"], "custom_json")
        self.assertEqual(cfg.standard_hours_per_day, 7.5)
        self.assertEqual(cfg.no_punch_mark, "未打卡")
        self.assertEqual(cfg.department_rules[0].group_name, "测试部门组")
        self.assertEqual(cfg.department_rules[0].match_value, "测试部")

    def test_custom_file_roundtrip(self) -> None:
        base = get_default_config()
        base.standard_hours_per_day = 6.5
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / "rules.xlsx"
            save_config(base, str(path))
            cfg, meta = resolve_overtime_config({"rules_file": str(path)})
            self.assertEqual(meta["source"], "custom_file")
            self.assertEqual(cfg.standard_hours_per_day, 6.5)

    def test_preview_dict_shape(self) -> None:
        cfg = config_from_dict(None)
        preview = config_to_preview_dict(cfg)
        self.assertIn("premium_rules", preview)
        self.assertIn("department_rules", preview)
        self.assertIn("params", preview)
        self.assertTrue(preview["department_rules"][0]["group_name"])


if __name__ == "__main__":
    unittest.main()
