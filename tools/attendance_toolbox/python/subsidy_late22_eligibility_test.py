# -*- coding: utf-8 -*-
"""晚走补贴资格判断单元测试。

覆盖业务规则的五类人员：
1. 外包人员 + 任意部门/岗位 → 无晚走补贴（最高优先级，不可被强制纳入名单覆盖）。
2. 正式员工 + 运营支撑部 + 运维工程师 → 无晚走补贴。
3. 正式员工 + 运营支撑部 + 客服 → 无晚走补贴。
4. 正式员工 + 运营支撑部 + 其他岗位 → 按现有规则计算晚走补贴（不排除）。
5. 正式员工 + 其他部门 → 保持现有规则（客服/售后/销售排除，研发等不排除）。

同时覆盖：
- 归一化（前后空格）；
- 强制纳入名单仅对非外包人员生效；
- 外包判定使用精确枚举（「非外包」不误判，「劳务派遣」不纳入）。
"""
from __future__ import annotations

import importlib.util
import unittest
from pathlib import Path

MODULE_PATH = Path(__file__).resolve().parent / "subsidy" / "calc_subsidy_deduction.py"


def _load_module():
    spec = importlib.util.spec_from_file_location("calc_subsidy_deduction_eligibility_test", MODULE_PATH)
    module = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(module)
    return module


class Late22EligibilityTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.module = _load_module()

    def _record(self, **overrides) -> dict:
        record = {
            "name": "测试人员",
            "dept1": "总部",
            "dept2": "研发中心",
            "dept3": None,
            "pos": "工程师",
            "emp_type": "正式员工",
        }
        record.update(overrides)
        return record

    # ── 规则 1：外包人员一律没有晚走补贴 ────────────────────────────────
    def test_outsourced_contractor_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(emp_type="外包", dept1="总部", dept2="研发中心", pos="工程师")
            )
        )

    def test_outsourced_ignores_dept_and_position(self) -> None:
        # 外包人员即使部门/岗位在允许范围内，仍应排除
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(emp_type="外包", dept1="运营管理中心", dept2="运营支撑部", pos="运维工程师")
            )
        )

    def test_outsourced_trim_space_normalized(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(self._record(emp_type=" 外包 "))
        )

    # ── 规则 2：运营支撑部 + 运维工程师 → 无晚走补贴 ─────────────────────
    def test_operations_engineer_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="运营管理中心", dept2="运营支撑部", pos="运维工程师")
            )
        )

    # ── 规则 3：运营支撑部 + 客服 → 无晚走补贴 ─────────────────────────
    def test_operations_customer_service_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="运营管理中心", dept2="运营支撑部", pos="客服")
            )
        )

    # ── 规则 4：运营支撑部 + 其他岗位 → 有晚走补贴 ───────────────────────
    def test_operations_other_position_included(self) -> None:
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(dept1="运营管理中心", dept2="运营支撑部", pos="运营专员")
            )
        )

    def test_operations_position_trim_space_normalized(self) -> None:
        # 前后空格不应导致误判
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="运营管理中心", dept2="运营支撑部", pos=" 运维工程师 ")
            )
        )

    # ── 规则 5：其他部门保持现有规则 ─────────────────────────────────────
    def test_other_dept_rd_not_excluded(self) -> None:
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(dept1="总部", dept2="研发中心", pos="工程师")
            )
        )

    def test_other_dept_customer_service_dept_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="总部", dept2="客服部", pos="专员")
            )
        )

    def test_other_dept_after_sales_dept_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="总部", dept2="售后部", pos="专员")
            )
        )

    def test_other_dept_sales_position_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="总部", dept2="业务部", pos="销售")
            )
        )

    def test_other_dept_sales_group_full_dept_excluded(self) -> None:
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(dept1="AI智慧文创事业部", dept2="销售组", pos="专员")
            )
        )

    # ── 强制纳入名单优先级高于排除规则 ─────────────────────────────────────
    def test_included_name_overrides_operations_exclusion(self) -> None:
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(name="崔利华", dept1="运营管理中心", dept2="运营支撑部", pos="运维工程师")
            )
        )

    def test_included_name_cannot_override_outsource_exclusion(self) -> None:
        # 外包是最高优先级，强制纳入名单不能覆盖：仍应排除。
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(name="崔利华", emp_type="外包")
            )
        )

    def test_included_name_overrides_other_dept_exclusion(self) -> None:
        # 正式员工 + 强制纳入名单 → 保留原有强制纳入效果。
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(name="郑楠杰", dept1="总部", dept2="客服部", pos="专员")
            )
        )

    # ── 外包判定使用精确枚举，避免误判 ─────────────────────────────────────
    def test_non_outsource_value_not_misjudged(self) -> None:
        # 员工类型为「非外包」时，精确集合匹配不会将其误判为外包。
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(emp_type="非外包", dept1="总部", dept2="研发中心", pos="工程师")
            )
        )

    def test_labor_dispatch_not_treated_as_outsource(self) -> None:
        # 「劳务派遣」未获业务确认属于外包范围，不应按外包排除（按部门岗位规则判断）。
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(emp_type="劳务派遣", dept1="总部", dept2="研发中心", pos="工程师")
            )
        )

    def test_outsourced_operations_other_position_still_excluded(self) -> None:
        # 外包人员 + 运营支撑部其他岗位 → 外包优先级更高，仍然无补贴。
        self.assertTrue(
            self.module._should_exclude_late22_count(
                self._record(
                    emp_type="外包",
                    dept1="运营管理中心",
                    dept2="运营支撑部",
                    pos="运营专员",
                )
            )
        )

    # ── 员工类型缺失/为空时按部门岗位判断 ─────────────────────────────────
    def test_missing_emp_type_falls_through_to_dept_rules(self) -> None:
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(emp_type=None, dept1="总部", dept2="研发中心", pos="工程师")
            )
        )

    def test_empty_emp_type_falls_through_to_dept_rules(self) -> None:
        self.assertFalse(
            self.module._should_exclude_late22_count(
                self._record(emp_type="", dept1="运营管理中心", dept2="运营支撑部", pos="运营专员")
            )
        )


if __name__ == "__main__":
    unittest.main()
