from __future__ import annotations

import argparse
import contextlib
import io
import json
import os
import re
import sys
import traceback
from datetime import date
from pathlib import Path


BASE_DIR = Path(__file__).resolve().parent
# Add BASE_DIR for dingtalk_sync.py which lives at the root level
if str(BASE_DIR) not in sys.path:
    sys.path.insert(0, str(BASE_DIR))
for subdir in ("leave", "overtime", "subsidy", "finally", "parttime"):
    path = str(BASE_DIR / subdir)
    if path not in sys.path:
        sys.path.insert(0, path)

import calc_leave
import fill_overtime_fields as ot
from rules_engine import get_default_config
import calc_subsidy_deduction as sub
import calc_finally as fin
import calc_parttime_summary as part
import dingtalk_sync
from excel_compat import audit_upload
import templates


NAME_SEPARATOR_RE = re.compile(r"[,，、;；\r\n]+")


def path_or_empty(config: dict, key: str) -> str:
    value = str(config.get(key) or "").strip()
    return value if value and os.path.exists(value) else ""


def paths(config: dict, key: str) -> list[str]:
    result = []
    for value in config.get(key) or []:
        value = str(value or "").strip()
        if value and os.path.exists(value):
            result.append(value)
    return result


def names(config: dict, key: str) -> list[str]:
    raw = config.get(key)
    if isinstance(raw, str):
        return [item.strip() for item in NAME_SEPARATOR_RE.split(raw) if item.strip()]
    if isinstance(raw, list):
        result = []
        for value in raw:
            result.extend(item.strip() for item in NAME_SEPARATOR_RE.split(str(value)) if item.strip())
        return result
    return []


def names_or_default(config: dict, key: str, default: list[str] | tuple[str, ...] | set[str]) -> list[str]:
    if key in config:
        return names(config, key)
    return list(default)


def default_text_values() -> dict[str, list[str]]:
    return {
        "leave_special_names": list(calc_leave.DEFAULT_SPECIAL_EMPLOYEE_NAMES),
        "chengdu_schedule_names": list(calc_leave.DEFAULT_CHENGDU_WORK_LOCATION_NAMES),
        "sub_dept_keywords": list(sub.RD_DEPT_KEYWORDS),
        "sub_late22_names": list(sub.LATE22_INCLUDED_NAMES),
        "part_special_names": list(part.DEFAULT_SPECIAL_DEFAULT_NAMES),
    }


def print_json(payload: dict) -> None:
    print(json.dumps(payload))


def capture(fn, *args, **kwargs):
    buf = io.StringIO()
    with contextlib.redirect_stdout(buf):
        result = fn(*args, **kwargs)
    return result, buf.getvalue()


def merge_overtime_schedule_map(target: dict, source: dict) -> None:
    for name, group_map in (source or {}).items():
        target_group = target.setdefault(name, {})
        for group_name, dates in (group_map or {}).items():
            target_group.setdefault(group_name, set()).update(dates or set())


def run_leave(config: dict, output_dir: Path) -> list[dict]:
    export_path = path_or_empty(config, "leave_src")
    schedule_path = path_or_empty(config, "leave_schedule")
    if not export_path or not schedule_path:
        raise ValueError("请上传请假系统导出表和作息表。")

    out_path = output_dir / "请假明细表.xlsx"
    src_rows, _ = capture(calc_leave.clean_export, export_path)
    schedule_ctx, _ = capture(calc_leave.load_schedule_context, schedule_path)
    offsite_overrides = None
    offsite_path = path_or_empty(config, "leave_offsite_duration")
    if offsite_path:
        offsite_overrides, _ = capture(calc_leave.load_offsite_duration_overrides, offsite_path)
    _, _ = capture(
        calc_leave.process,
        src_rows,
        str(out_path),
        schedule_ctx,
        None,
        offsite_overrides,
        tuple(names_or_default(config, "leave_special_names", calc_leave.DEFAULT_SPECIAL_EMPLOYEE_NAMES)),
        tuple(names_or_default(config, "chengdu_schedule_names", calc_leave.DEFAULT_CHENGDU_WORK_LOCATION_NAMES)),
    )
    return [{"path": str(out_path), "file_name": out_path.name}]


def run_overtime(config: dict, output_dir: Path) -> list[dict]:
    export_path = path_or_empty(config, "overtime_src")
    if not export_path:
        raise ValueError("请上传加班系统导出表。")

    out_path = output_dir / "加班明细_回填.xlsx"
    rules_config = get_default_config()
    attendance_path = path_or_empty(config, "overtime_attendance")
    calendar_path = path_or_empty(config, "overtime_calendar")
    roster_path = path_or_empty(config, "overtime_roster")
    schedule_paths = paths(config, "overtime_schedules")

    name_group_map = None
    if attendance_path:
        name_group_map, _ = capture(ot.load_attendance_name_group_map_if_available, attendance_path)

    employee_department_map = {}
    if roster_path:
        employee_department_map, _ = capture(ot.parse_employee_department_map, roster_path)
        if not employee_department_map:
            raise ValueError("花名册/员工信息表未识别到可用部门映射。")

    src_rows, _ = capture(ot.clean_export_overtime, export_path)
    _, _ = capture(ot.load_work_calendar, calendar_path, rules_config)
    target_ym = ot.get_work_calendar_month_key() or ot._infer_target_month_key(src_rows)

    schedule_map = {}
    if schedule_paths:
        for schedule_path in schedule_paths:
            loaded_result, _ = capture(
                ot.load_schedule_if_available,
                schedule_path,
                None,
                target_ym,
                set(),
                rules_config,
            )
            single_map, loaded = loaded_result
            if loaded:
                merge_overtime_schedule_map(schedule_map, single_map)
    else:
        loaded_result, _ = capture(ot.load_schedule_if_available, "", None, target_ym, set(), rules_config)
        schedule_map, _ = loaded_result

    attendance_missing_checkout_map, _ = capture(ot.load_attendance_if_available, attendance_path)
    attendance_missing_checkout_map, _ = attendance_missing_checkout_map
    _, _ = capture(
        ot.process_overtime,
        src_rows,
        str(out_path),
        schedule_map,
        attendance_missing_checkout_map,
        name_group_map,
        rules_config,
        None,
        tuple(names_or_default(config, "chengdu_schedule_names", calc_leave.DEFAULT_CHENGDU_WORK_LOCATION_NAMES)),
        employee_department_map,
    )
    return [{"path": str(out_path), "file_name": out_path.name}]


def run_subsidy(config: dict, output_dir: Path) -> list[dict]:
    src_path = path_or_empty(config, "subsidy_src")
    checkin_path = path_or_empty(config, "subsidy_checkin")
    schedule_path = path_or_empty(config, "subsidy_schedule")
    if not src_path or not checkin_path or not schedule_path:
        raise ValueError("请上传补贴扣款表、签到表和作息表。")

    att_path = path_or_empty(config, "subsidy_attendance") or src_path
    att_result_path = path_or_empty(config, "subsidy_attendance_result")
    if att_path == src_path and not sub.looks_like_attendance_summary(src_path):
        raise ValueError("未上传考勤表，且补贴扣款表中未识别到日期级考勤结果列。")

    rd_keywords = names_or_default(config, "sub_dept_keywords", sub.RD_DEPT_KEYWORDS)
    late22_names = names_or_default(config, "sub_late22_names", sub.LATE22_INCLUDED_NAMES)
    out_path = output_dir / "补贴扣款表_核对.xlsx"
    diff_path = output_dir / "补贴扣款表_差异清单.xlsx"

    period_year, period_month, period_source = sub.resolve_schedule_period(schedule_path)
    source_records = sub.parse_source_table(src_path, rd_dept_keywords=rd_keywords)
    activity_days = sub.parse_activity_checkin(checkin_path)
    employees = sub.parse_attendance(
        att_path,
        rd_dept_keywords=rd_keywords,
        year=period_year,
        month=period_month,
        period_source=period_source,
    )
    legal_holidays = sub.load_statutory_holidays_from_schedule(
        schedule_path,
        target_year=period_year,
        target_month=period_month,
    )
    approved_dates_by_name = None
    if att_result_path:
        approved_dates_by_name = sub.parse_attendance_result(
            att_result_path,
            year=period_year,
            month=period_month,
        )

    mismatches, _ = sub.write_output(
        src_path,
        source_records,
        employees,
        activity_days,
        str(out_path),
        approved_dates_by_name=approved_dates_by_name,
        legal_holidays=legal_holidays,
        late22_included_names=late22_names,
    )
    outputs = [{"path": str(out_path), "file_name": out_path.name}]
    if mismatches:
        sub.write_mismatch_report(mismatches, str(diff_path))
        outputs.append({"path": str(diff_path), "file_name": diff_path.name})
    return outputs


def run_final(config: dict, output_dir: Path) -> list[dict]:
    active_path = path_or_empty(config, "final_active")
    schedule_path = path_or_empty(config, "final_schedule")
    leave_path = path_or_empty(config, "final_leave")
    overtime_path = path_or_empty(config, "final_overtime")
    subsidy_path = path_or_empty(config, "final_subsidy")
    if not all([active_path, schedule_path, leave_path, overtime_path, subsidy_path]):
        raise ValueError("请上传在职花名册、作息表、请假明细表、加班明细表和补贴扣款表。")

    out_path = output_dir / "最终表.xlsx"
    resign_path = path_or_empty(config, "final_resign") or None
    transfer_path = path_or_empty(config, "final_transfer")

    employees = fin.parse_roster(active_path, resign_path)
    transfer_map = fin.parse_transfer(transfer_path) if transfer_path else {}
    schedule_ctx = fin.parse_schedule(schedule_path)
    chengdu_names = tuple(names_or_default(config, "chengdu_schedule_names", calc_leave.DEFAULT_CHENGDU_WORK_LOCATION_NAMES))
    leave_map = fin.parse_leave_summary(leave_path, schedule_ctx)
    leave_day_details = fin.parse_leave_day_details(leave_path, schedule_ctx, chengdu_names)
    overtime_map = fin.parse_overtime_summary(
        overtime_path,
        schedule_ctx["year"],
        schedule_ctx["month"],
        employees,
        schedule_ctx,
    )
    absent_by_no, absent_by_name = fin.parse_subsidy_absent(subsidy_path)
    absent_day_details = fin.parse_subsidy_absent_day_details(
        subsidy_path,
        schedule_ctx["year"],
        schedule_ctx["month"],
    )
    fin.generate(
        employees,
        transfer_map,
        schedule_ctx,
        leave_map,
        overtime_map,
        absent_by_no,
        absent_by_name,
        str(out_path),
        leave_day_details,
        chengdu_names,
        absent_day_details,
    )
    return [{"path": str(out_path), "file_name": out_path.name}]


def run_parttime(config: dict, output_dir: Path) -> list[dict]:
    out_path = output_dir / "兼职汇总.xlsx"
    default_schedule_path = path_or_empty(config, "parttime_default_schedule")
    if not default_schedule_path:
        raise ValueError("请上传默认作息表。")
    part.generate_parttime_summary(
        output_path=str(out_path),
        attendance_detail_path=path_or_empty(config, "parttime_attendance_detail") or None,
        monthly_summary_paths=paths(config, "parttime_monthly"),
        schedule_paths=paths(config, "parttime_schedules"),
        default_schedule_path=default_schedule_path,
        special_default_names=names_or_default(config, "part_special_names", part.DEFAULT_SPECIAL_DEFAULT_NAMES),
    )
    return [{"path": str(out_path), "file_name": out_path.name}]


def run_dingtalk_sync(config: dict, output_dir: Path) -> list[dict]:
    """Run DingTalk sync and generate intermediate tables."""
    from datetime import datetime

    # Parse config parameters
    start_date_str = config.get("dingtalk_sync_start_date")
    end_date_str = config.get("dingtalk_sync_end_date")
    if not start_date_str or not end_date_str:
        raise ValueError("请提供同步开始日期和结束日期。")

    start_date = datetime.strptime(str(start_date_str), "%Y-%m-%d").date()
    end_date = datetime.strptime(str(end_date_str), "%Y-%m-%d").date()

    # Flow keys: leave, overtime, attendance_correction, position_transfer
    flow_keys = config.get("dingtalk_sync_flow_keys") or ["leave", "overtime", "attendance_correction", "position_transfer"]
    if isinstance(flow_keys, str):
        flow_keys = [k.strip() for k in flow_keys.split(",") if k.strip()]

    # Max instances per flow (None means unlimited)
    max_instances = config.get("dingtalk_sync_max_instances")
    if max_instances is not None:
        max_instances = int(max_instances)

    # Query window padding days
    padding_days = int(config.get("dingtalk_sync_padding_days", 31))

    # DingTalk config from environment or config
    dingtalk_config = dingtalk_sync.config_from_mapping(config.get("dingtalk", {}))

    # Run sync
    result = dingtalk_sync.sync_date_range(
        dingtalk_config,
        start_date,
        end_date,
        max_instances_per_flow=max_instances,
        flow_keys=flow_keys,
        query_window_padding_days=padding_days,
    )

    # Write exports to output_dir
    outputs = []
    for key, export_data in result.exports.items():
        if export_data and export_data.data:
            out_path = output_dir / export_data.file_name
            out_path.write_bytes(export_data.data)
            outputs.append({"path": str(out_path), "file_name": export_data.file_name})

    # Write audit report if available
    if result.audit_export and result.audit_export.data:
        audit_path = output_dir / result.audit_export.file_name
        audit_path.write_bytes(result.audit_export.data)
        outputs.append({"path": str(audit_path), "file_name": result.audit_export.file_name})

    return outputs


RUNNERS = {
    "leave": run_leave,
    "overtime": run_overtime,
    "subsidy": run_subsidy,
    "final": run_final,
    "parttime": run_parttime,
    "dingtalk_sync": run_dingtalk_sync,
}


# ── Action: export-rules ─────────────────────────────────────────────────────
# 将加班规则配置导出为 Excel 文件

def action_export_rules(config: dict, output_dir: Path) -> list[dict]:
    from rules_engine import save_config

    out_path = output_dir / "加班规则配置.xlsx"
    rules_config = get_default_config()
    # 允许通过 config 覆盖默认配置中的 department_rules
    if "rules_department_rules" in config:
        # 简单覆盖：从 config 重建 department_rules
        pass  # 默认配置已经包含所有规则
    save_config(rules_config, str(out_path))
    return [{"path": str(out_path), "file_name": out_path.name}]


# ── Action: import-rules-preview ─────────────────────────────────────────────
# 导入加班规则 Excel，返回 JSON 预览（不写入，只解析）

def action_import_rules_preview(config: dict, output_dir: Path) -> list[dict]:
    from rules_engine import load_config, load_config_or_default

    rules_path = path_or_empty(config, "rules_file")
    if not rules_path:
        raise ValueError("请上传加班规则配置文件。")

    loaded_config = load_config_or_default(rules_path)
    preview = {
        "premium_rules": [
            {
                "date_type": r.date_type,
                "department_group": r.department_group,
                "multiplier": r.multiplier,
                "action": r.action,
            }
            for r in loaded_config.premium_rules
        ],
        "department_rules": [
            {
                "level": r.level,
                "match_type": r.match_type,
                "keywords": list(r.keywords),
                "group": r.group,
            }
            for r in loaded_config.department_rules
        ],
        "params": {
            "rest_premium_excluded_names": list(loaded_config.rest_premium_excluded_names),
            "rest_premium_excluded_codes": list(loaded_config.rest_premium_excluded_codes),
        },
    }
    # 写入 preview JSON 文件供 Go 层读取
    preview_path = output_dir / "rules_preview.json"
    preview_path.write_text(json.dumps(preview, ensure_ascii=False, indent=2), encoding="utf-8")
    return [{"path": str(preview_path), "file_name": preview_path.name}]


# ── Action: validate ─────────────────────────────────────────────────────────
# 校验上传的 Excel 文件表头是否匹配预期

EXPECTED_HEADERS = {
    "leave": {
        "export": ["姓名", "工号", "开始时间", "结束时间", "请假类型", "审批状态"],
        "schedule": ["日期"],
    },
    "overtime": {
        "export": ["姓名", "工号", "开始时间", "结束时间"],
    },
    "subsidy": {
        "source": ["姓名", "工号"],
    },
    "final": {
        "roster": ["姓名", "工号"],
        "schedule": ["日期"],
    },
    "parttime": {
        "detail": ["姓名", "工号"],
    },
}


def _check_headers(file_path: str, expected: list[str], sheet_index: int = 0) -> dict:
    """检查 Excel 文件的表头是否包含预期列。"""
    try:
        import openpyxl
        wb = openpyxl.load_workbook(file_path, read_only=True, data_only=True)
        ws = wb.worksheets[sheet_index]
        headers = [str(cell.value or "").strip() for cell in next(ws.iter_rows(min_row=1, max_row=1))]
        wb.close()
        missing = [h for h in expected if h not in headers]
        return {"ok": len(missing) == 0, "headers": headers, "missing": missing}
    except Exception as exc:
        return {"ok": False, "error": str(exc)}


def action_validate(config: dict, output_dir: Path) -> list[dict]:
    module = config.get("validate_module", "")
    results = {}

    if module == "leave":
        export_path = path_or_empty(config, "leave_src")
        schedule_path = path_or_empty(config, "leave_schedule")
        if export_path:
            results["leave_export"] = _check_headers(export_path, EXPECTED_HEADERS["leave"]["export"])
        if schedule_path:
            results["leave_schedule"] = _check_headers(schedule_path, EXPECTED_HEADERS["leave"]["schedule"])
    elif module == "overtime":
        export_path = path_or_empty(config, "overtime_src")
        if export_path:
            results["overtime_export"] = _check_headers(export_path, EXPECTED_HEADERS["overtime"]["export"])
    elif module == "subsidy":
        src_path = path_or_empty(config, "subsidy_src")
        if src_path:
            results["subsidy_source"] = _check_headers(src_path, EXPECTED_HEADERS["subsidy"]["source"])
    elif module == "final":
        roster_path = path_or_empty(config, "final_active")
        schedule_path = path_or_empty(config, "final_schedule")
        if roster_path:
            results["final_roster"] = _check_headers(roster_path, EXPECTED_HEADERS["final"]["roster"])
        if schedule_path:
            results["final_schedule"] = _check_headers(schedule_path, EXPECTED_HEADERS["final"]["schedule"])
    elif module == "parttime":
        detail_path = path_or_empty(config, "parttime_attendance_detail")
        if detail_path:
            results["parttime_detail"] = _check_headers(detail_path, EXPECTED_HEADERS["parttime"]["detail"])

    all_ok = all(r.get("ok", False) for r in results.values()) if results else True
    preview_path = output_dir / "validation_result.json"
    preview_path.write_text(json.dumps({"ok": all_ok, "results": results}, ensure_ascii=False, indent=2), encoding="utf-8")
    return [{"path": str(preview_path), "file_name": preview_path.name}]


# ── Action: preview ──────────────────────────────────────────────────────────
# 运行计算，但只返回前 200 行 JSON 预览

def _xlsx_to_preview_json(xlsx_path: str, max_rows: int = 200) -> list[dict]:
    """将 Excel 文件转换为前 N 行的 JSON 列表。"""
    import openpyxl
    wb = openpyxl.load_workbook(xlsx_path, read_only=True, data_only=True)
    ws = wb.worksheets[0]
    rows_iter = ws.iter_rows(values_only=True)
    headers = [str(h or f"col_{i}") for i, h in enumerate(next(rows_iter, []))]
    data = []
    for i, row in enumerate(rows_iter):
        if i >= max_rows:
            break
        data.append({headers[j]: str(cell) if cell is not None else "" for j, cell in enumerate(row)})
    wb.close()
    return data


def action_preview(config: dict, output_dir: Path) -> list[dict]:
    module = config.get("preview_module") or config.get("module", "")
    if not module or module not in RUNNERS:
        raise ValueError(f"未知模块：{module}")

    # 运行实际计算，生成输出文件
    outputs = RUNNERS[module](config, output_dir)

    # 读取第一个输出文件的前 200 行
    preview_data = []
    if outputs:
        first_output = outputs[0]["path"]
        if first_output.endswith(".xlsx"):
            preview_data = _xlsx_to_preview_json(first_output, max_rows=200)

    preview_path = output_dir / "preview.json"
    preview_path.write_text(
        json.dumps({"ok": True, "preview": preview_data, "total_outputs": len(outputs)}, ensure_ascii=False, indent=2),
        encoding="utf-8",
    )
    return [{"path": str(preview_path), "file_name": preview_path.name}] + outputs


# ── Action: export-templates ─────────────────────────────────────────────────
# 输出工具箱用到的 Excel 空白模板（单个或全部 zip）


def action_export_templates(config: dict, output_dir: Path) -> list[dict]:
    """Export one template or the full template set.

    config:
        template_id: optional TEMPLATE_REGISTRY id; if missing, returns all 16.
        templates_meta: optional bool (default true) — include template list.
    """
    requested = config.get("template_id")
    requested_str = str(requested).strip() if requested is not None else ""

    if requested_str:
        if requested_str not in templates.TEMPLATE_BY_ID:
            raise ValueError(f"未知模板：{requested_str}")
        filename, builder = templates.TEMPLATE_BY_ID[requested_str]
        data = builder()
        out_path = output_dir / filename
        out_path.write_bytes(data)
        outputs = [{"path": str(out_path), "file_name": filename}]
    else:
        outputs = []
        for template_id, filename, builder in templates.TEMPLATE_REGISTRY:
            data = builder()
            out_path = output_dir / filename
            out_path.write_bytes(data)
            outputs.append({"path": str(out_path), "file_name": filename})

    if str(config.get("templates_meta", "true")).lower() in {"true", "1", "yes"}:
        meta_path = output_dir / "templates_meta.json"
        meta_path.write_text(
            json.dumps(
                {
                    "templates": [
                        {"id": tid, "file_name": fname}
                        for tid, fname, _ in templates.TEMPLATE_REGISTRY
                    ],
                    "count": len(templates.TEMPLATE_REGISTRY),
                },
                ensure_ascii=False,
                indent=2,
            ),
            encoding="utf-8",
        )
        outputs.append({"path": str(meta_path), "file_name": meta_path.name})

    return outputs


# ── Action: audit ─────────────────────────────────────────────────────────────
# 接收文件名列表和对应的字节数，返回观测警告（支持扫描内容）

MAX_AUDIT_BYTES = 1 * 1024 * 1024  # 内容扫描时最多读取的前 N 字节


def action_audit(config: dict, output_dir: Path) -> list[dict]:
    """Audit uploaded files before running computation.

    config shape (all fields optional):
        files: list of { "name": str, "size": int, "path": str (absolute, on disk) }
        max_warn_mb: int (default 50)
        max_warn_rows: int (default 50000)
        max_warn_cols: int (default 500)
        scan_content: bool (default true)
    """
    files_meta = config.get("files") or []
    if isinstance(files_meta, dict):
        files_meta = files_meta.get("items") or []
    if not isinstance(files_meta, list):
        raise ValueError("files 字段需要是 list")

    max_warn_mb = int(config.get("max_warn_mb", 50))
    max_warn_rows = int(config.get("max_warn_rows", 50000))
    max_warn_cols = int(config.get("max_warn_cols", 500))
    scan_content = str(config.get("scan_content", "true")).lower() not in {"0", "false", "no"}

    all_warnings: list[str] = []
    per_file: list[dict] = []
    audit_log: list[dict] = []

    for raw in files_meta:
        if not isinstance(raw, dict):
            continue
        name = str(raw.get("name") or raw.get("file_name") or "uploaded file")
        size = int(raw.get("size") or raw.get("size_bytes") or 0)
        raw_path = str(raw.get("path") or "").strip()

        data = b""
        if raw_path and os.path.exists(raw_path):
            try:
                with open(raw_path, "rb") as fh:
                    data = fh.read(MAX_AUDIT_BYTES)
            except Exception:
                data = b""
        elif "bytes" in raw:
            supplied = raw["bytes"]
            if isinstance(supplied, (bytes, bytearray)):
                data = bytes(supplied[:MAX_AUDIT_BYTES])
            elif isinstance(supplied, str):
                try:
                    data = __import__("base64").b64decode(supplied)[:MAX_AUDIT_BYTES]
                except Exception:
                    data = b""

        result = audit_upload(
            name,
            size,
            data,
            max_warn_mb=max_warn_mb,
            max_warn_rows=max_warn_rows,
            max_warn_cols=max_warn_cols,
        )
        all_warnings.extend(result.get("warnings", []))
        audit_log.extend(result.get("audit", []))
        per_file.append({"file": result.get("file_name", name), "warnings": result.get("warnings", [])})

    out = {
        "ok": True,
        "warnings": all_warnings,
        "audit": audit_log,
        "files": per_file,
        "thresholds": {
            "max_warn_mb": max_warn_mb,
            "max_warn_rows": max_warn_rows,
            "max_warn_cols": max_warn_cols,
            "scan_content": scan_content,
        },
    }
    audit_path = output_dir / "audit_result.json"
    audit_path.write_text(json.dumps(out, ensure_ascii=False, indent=2), encoding="utf-8")
    return [{"path": str(audit_path), "file_name": audit_path.name}]


ACTIONS = {
    "export-rules": action_export_rules,
    "import-rules-preview": action_import_rules_preview,
    "validate": action_validate,
    "preview": action_preview,
    "audit": action_audit,
    "export-templates": action_export_templates,
}


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--module", choices=sorted(RUNNERS))
    parser.add_argument("--action", choices=sorted(ACTIONS))
    parser.add_argument("--workdir")
    parser.add_argument("--defaults", action="store_true")
    parser.add_argument("--config-json", default="{}")
    args = parser.parse_args()

    if args.defaults:
        print_json({
            "ok": True,
            "defaults": default_text_values(),
        })
        return 0

    # Actions that don't require --module
    if args.action and args.action in ACTIONS:
        if not args.workdir:
            parser.error("--workdir is required for --action")
        workdir = Path(args.workdir).resolve()
        output_dir = workdir / "outputs"
        output_dir.mkdir(parents=True, exist_ok=True)
        try:
            config = json.loads(args.config_json)
            log_buffer = io.StringIO()
            with contextlib.redirect_stdout(log_buffer):
                outputs = ACTIONS[args.action](config, output_dir)
            print_json({
                "ok": True,
                "outputs": outputs,
                "log": log_buffer.getvalue(),
            })
            return 0
        except Exception as exc:
            print_json({
                "ok": False,
                "error": str(exc),
                "traceback": traceback.format_exc(),
            })
            return 1

    if not args.module or not args.workdir:
        parser.error("--module and --workdir are required unless --defaults or --action is used")

    workdir = Path(args.workdir).resolve()
    output_dir = workdir / "outputs"
    output_dir.mkdir(parents=True, exist_ok=True)

    try:
        config = json.loads(args.config_json)
        log_buffer = io.StringIO()
        with contextlib.redirect_stdout(log_buffer):
            outputs = RUNNERS[args.module](config, output_dir)
        print_json({
            "ok": True,
            "outputs": outputs,
            "log": log_buffer.getvalue(),
        })
        return 0
    except Exception as exc:
        print_json({
            "ok": False,
            "error": str(exc),
            "traceback": traceback.format_exc(),
        })
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
