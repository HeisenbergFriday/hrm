# -*- coding: utf-8 -*-
"""Adapter only: build OvertimeConfig from JSON/dict without changing rules_engine matching."""

from __future__ import annotations

import json
from datetime import date, datetime
from typing import Any

from rules_engine import (
    DepartmentRule,
    OvertimeConfig,
    PremiumRule,
    get_default_config,
    load_config,
)


def _as_bool(value: Any, default: bool = False) -> bool:
    if value is None:
        return default
    if isinstance(value, bool):
        return value
    text = str(value).strip().lower()
    if text in {"1", "true", "yes", "y", "是", "on"}:
        return True
    if text in {"0", "false", "no", "n", "否", "off"}:
        return False
    return default


def _as_float(value: Any, default: float) -> float:
    try:
        if value is None or value == "":
            return default
        return float(value)
    except (TypeError, ValueError):
        return default


def _as_int(value: Any, default: int = 0) -> int:
    try:
        if value is None or value == "":
            return default
        return int(value)
    except (TypeError, ValueError):
        return default


def _parse_date(value: Any) -> date | None:
    if value is None or value == "":
        return None
    if isinstance(value, datetime):
        return value.date()
    if isinstance(value, date):
        return value
    text = str(value).strip()
    if not text:
        return None
    for fmt in ("%Y-%m-%d", "%Y/%m/%d", "%Y.%m.%d"):
        try:
            return datetime.strptime(text[:10], fmt).date()
        except ValueError:
            continue
    try:
        return date.fromisoformat(text[:10])
    except ValueError:
        return None


def config_from_dict(data: dict[str, Any] | None) -> OvertimeConfig:
    """Build OvertimeConfig from frontend/export JSON. Falls back to defaults for missing fields."""
    base = get_default_config()
    if not data:
        return base

    premium_rules: list[PremiumRule] = []
    for raw in data.get("premium_rules") or []:
        if not isinstance(raw, dict):
            continue
        premium_rules.append(
            PremiumRule(
                priority=_as_int(raw.get("priority"), 99),
                date_type=str(raw.get("date_type") or "").strip(),
                department_group=str(raw.get("department_group") or "全部").strip() or "全部",
                action=str(raw.get("action") or "").strip(),
                multiplier=_as_float(raw.get("multiplier"), 0.0),
            )
        )
    if not premium_rules:
        premium_rules = list(base.premium_rules)
    premium_rules.sort(key=lambda r: r.priority)

    department_rules: list[DepartmentRule] = []
    for raw in data.get("department_rules") or []:
        if not isinstance(raw, dict):
            continue
        # Support both current schema and any legacy keys if present.
        group_name = str(
            raw.get("group_name") or raw.get("group") or ""
        ).strip()
        match_field = str(
            raw.get("match_field") or raw.get("level") or ""
        ).strip()
        match_method = str(
            raw.get("match_method") or raw.get("match_type") or "包含"
        ).strip() or "包含"
        match_value = str(raw.get("match_value") or "").strip()
        if not match_value and raw.get("keywords") is not None:
            keywords = raw.get("keywords")
            if isinstance(keywords, (list, tuple)):
                match_value = ",".join(str(k).strip() for k in keywords if str(k).strip())
            else:
                match_value = str(keywords).strip()
        if not group_name and not match_value:
            continue
        department_rules.append(
            DepartmentRule(
                group_name=group_name,
                match_field=match_field,
                match_method=match_method,
                match_value=match_value,
            )
        )
    if not department_rules:
        department_rules = list(base.department_rules)

    params = data.get("params") if isinstance(data.get("params"), dict) else {}
    legal_override: set[date] = set()
    for item in data.get("legal_holidays_override") or []:
        if isinstance(item, dict):
            parsed = _parse_date(item.get("date") or item.get("日期"))
        else:
            parsed = _parse_date(item)
        if parsed is not None:
            legal_override.add(parsed)

    config = OvertimeConfig(
        premium_rules=premium_rules,
        department_rules=department_rules,
        standard_hours_per_day=_as_float(
            params.get("standard_hours_per_day"), base.standard_hours_per_day
        ),
        no_punch_mark=str(params.get("no_punch_mark") or base.no_punch_mark),
        schedule_augment_holidays=_as_bool(
            params.get("schedule_augment_holidays"), base.schedule_augment_holidays
        ),
        schedule_augment_rest_dept_group=str(
            params.get("schedule_augment_rest_dept_group")
            or base.schedule_augment_rest_dept_group
            or ""
        ),
        chengdu_use_separate_calendar=_as_bool(
            params.get("chengdu_use_separate_calendar"), base.chengdu_use_separate_calendar
        ),
        legal_holidays_override=legal_override or None,
    )
    config.build_lookups()
    return config


def config_summary(config: OvertimeConfig, source: str) -> dict[str, Any]:
    return {
        "source": source,
        "premium_rules_count": len(config.premium_rules),
        "department_rules_count": len(config.department_rules),
        "standard_hours_per_day": config.standard_hours_per_day,
        "no_punch_mark": config.no_punch_mark,
        "schedule_augment_holidays": config.schedule_augment_holidays,
        "schedule_augment_rest_dept_group": config.schedule_augment_rest_dept_group,
        "chengdu_use_separate_calendar": config.chengdu_use_separate_calendar,
        "legal_holidays_override_count": len(config.legal_holidays_override or set()),
    }


def config_to_preview_dict(config: OvertimeConfig) -> dict[str, Any]:
    return {
        "premium_rules": [
            {
                "priority": r.priority,
                "date_type": r.date_type,
                "department_group": r.department_group,
                "action": r.action,
                "multiplier": r.multiplier,
            }
            for r in config.premium_rules
        ],
        "department_rules": [
            {
                "group_name": r.group_name,
                "match_field": r.match_field,
                "match_method": r.match_method,
                "match_value": r.match_value,
            }
            for r in config.department_rules
        ],
        "params": {
            "standard_hours_per_day": config.standard_hours_per_day,
            "no_punch_mark": config.no_punch_mark,
            "schedule_augment_holidays": config.schedule_augment_holidays,
            "schedule_augment_rest_dept_group": config.schedule_augment_rest_dept_group,
            "chengdu_use_separate_calendar": config.chengdu_use_separate_calendar,
        },
        "legal_holidays_override": sorted(
            d.isoformat() for d in (config.legal_holidays_override or set())
        ),
    }


def resolve_overtime_config(config: dict[str, Any]) -> tuple[OvertimeConfig, dict[str, Any]]:
    """
    Resolve overtime rules for a runner config.

    Priority:
    1. rules_json (dict or JSON string)
    2. rules_file (xlsx path)
    3. default get_default_config()
    """
    rules_json = config.get("rules_json")
    if isinstance(rules_json, str) and rules_json.strip():
        try:
            rules_json = json.loads(rules_json)
        except json.JSONDecodeError as exc:
            raise ValueError(f"rules_json 不是合法 JSON: {exc}") from exc

    if isinstance(rules_json, dict) and rules_json:
        ot_config = config_from_dict(rules_json)
        return ot_config, config_summary(ot_config, "custom_json")

    rules_file = str(config.get("rules_file") or "").strip()
    if rules_file:
        ot_config = load_config(rules_file)
        return ot_config, config_summary(ot_config, "custom_file")

    ot_config = get_default_config()
    return ot_config, config_summary(ot_config, "default")
