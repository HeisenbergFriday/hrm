"""Shared preflight and export audit for the five attendance workflows."""
from __future__ import annotations

import calendar
import json
import re
from collections import defaultdict
from datetime import date, datetime
from pathlib import Path

import openpyxl
from openpyxl.styles import PatternFill
from excel_compat import load_workbook_compat

NAMES = {"姓名", "员工姓名", "人员姓名", "发起人姓名"}
CODES = {"工号", "员工工号", "员工编号"}
IGNORE_SHEETS = {"填写说明", "文件检查", "数据检查", "异常审计", "异常清单"}
MONTH_RE = re.compile(r"(?:(20\d{2})[年/_.-]?)?(\d{1,2})月|(?<!\d)(20\d{2})[-/.](\d{1,2})(?:[-/.]\d{1,2})?|(?<!\d)(20\d{2})(\d{2})(?!\d)")


def months(value):
    if isinstance(value, (date, datetime)):
        return {(value.year, value.month)}
    found = set()
    for match in MONTH_RE.finditer(str(value or "")):
        a, b, c, d, e, f = match.groups()
        year, month = int(a or c or e or 0), int(b or d or f)
        if 1 <= month <= 12:
            found.add((year, month))
    return found


def text(value):
    return re.sub(r"[\u200b-\u200d\ufeff]", "", str(value or "")).strip()


def issue(report, kind, person, detail, file_name=""):
    report["issues"].append({"kind": kind, "person": person, "detail": detail, "file_name": file_name})


def inspect_file(path, field, target, report):
    name = Path(path).name
    item = {"file_name": name, "field": field, "type": "待识别", "status": "正常", "detail": "", "people": []}
    start = len(report["issues"])
    roster = field.endswith(("_active", "_resign", "_roster"))
    approval = field in {"leave_src", "leave_reviewed", "leave_offsite_duration", "overtime_src", "final_leave", "final_overtime", "final_transfer"}
    calendar_file = field.endswith(("_calendar", "_default_schedule")) or field in {"leave_schedule", "subsidy_schedule", "final_schedule"}
    matrix = field.startswith("parttime_") or field.endswith(("_attendance", "_attendance_result", "_schedules"))
    wb = None
    try:
        wb = load_workbook_compat(path, data_only=True)
        hints = set(months(name)) if not roster and not approval else set()
        matched_sheets = 0
        for ws in wb.worksheets:
            if ws.title in IGNORE_SHEETS:
                continue
            header_row, headers = None, []
            for row_idx in range(1, min(ws.max_row, 15) + 1):
                row = [ws.cell(row_idx, col).value for col in range(1, ws.max_column + 1)]
                clean = [text(v) for v in row]
                if any(v in NAMES for v in clean):
                    header_row, headers = row_idx, clean
                    break
                if not roster and not approval:
                    for value in row:
                        hints.update(months(value))
            if calendar_file:
                item["type"] = "作息表"
                matched_sheets += 1
                continue
            if header_row is None:
                continue
            matched_sheets += 1
            item["type"] = "花名册" if roster else "审批明细" if approval else "排班表" if field.endswith("_schedules") else "考勤明细/月度汇总"
            name_col = next(i for i, h in enumerate(headers) if h in NAMES)
            code_col = next((i for i, h in enumerate(headers) if h in CODES), None)
            date_cols = [i for i, h in enumerate(headers) if any(word in h for word in ("日期", "时间", "计算月份"))]
            day_cols = [i for i, h in enumerate(headers) if re.fullmatch(r"(?:[1-9]|[12]\d|3[01])(?:日|号)?", h) or (not approval and months(h))]
            # DingTalk two-row date headers put day numbers below 考勤结果.
            if matrix and not day_cols and header_row < ws.max_row:
                day_cols = [i for i in range(ws.max_column) if re.fullmatch(r"(?:[1-9]|[12]\d|3[01])(?:日|号)?", text(ws.cell(header_row + 1, i + 1).value))]
            if not roster and not approval:
                for header in headers:
                    hints.update(months(header))
            if not roster and not approval and not date_cols and not day_cols and "考勤结果" not in headers and not any("出勤" in h or "补贴" in h or "扣款" in h for h in headers):
                issue(report, "缺少日期", "", "未找到日期列、逐日表头或月度统计字段", name)
            seen_codes, seen_names = set(), defaultdict(list)
            for row_idx in range(header_row + 1, ws.max_row + 1):
                values = [ws.cell(row_idx, col).value for col in range(1, ws.max_column + 1)]
                employee = text(values[name_col])
                code = text(values[code_col]).upper() if code_col is not None else ""
                if employee in {"姓名", "日期", "星期", "合计", "总计"}:
                    continue
                if not employee:
                    if code or (any(values[i] not in (None, "") for i in day_cols) and row_idx > header_row + 1):
                        issue(report, "空姓名", "", f"{ws.title} 第 {row_idx} 行姓名为空", name)
                    continue
                if employee.startswith("【示例】"):
                    issue(report, "模板示例", employee, "请删除示例行或替换为真实员工信息", name)
                if code and code in seen_codes and not approval:
                    issue(report, "重复工号", employee, f"{ws.title} 第 {row_idx} 行工号 {code} 重复", name)
                if code:
                    seen_codes.add(code)
                seen_names[employee].append(code)
                item["people"].append({"name": employee, "code": code})
                if matrix and day_cols and all(values[i] in (None, "") for i in day_cols):
                    issue(report, "整月空白", f"{code} {employee}".strip(), "没有逐日考勤记录", name)
                if not roster and not approval:
                    for i in date_cols:
                        hints.update(months(values[i]))
            for employee, codes in seen_names.items():
                if len(codes) > 1 and "" in codes and not approval:
                    issue(report, "同名待确认", employee, "同名记录缺少工号，请补齐工号后重试", name)
        if not matched_sheets:
            issue(report, "未识别文件", "", "未找到姓名列或受支持的文件结构", name)
        if not roster and not approval:
            if not hints:
                issue(report, "月份缺失", "", "文件未标明年月，请在表头补充处理月份", name)
            for year, month in hints:
                if (year or target[0], month) != target:
                    issue(report, "月份冲突", "", f"文件标明 {year or target[0]}-{month:02d}，处理月份为 {target[0]}-{target[1]:02d}", name)
        # Upstream reports with outstanding issues cannot enter final totals.
        if "数据检查" in wb.sheetnames:
            audit = wb["数据检查"]
            for row in audit.iter_rows(values_only=True):
                if any(text(v) in {"待处理", "待确认", "待核对", "needs_review", "失败"} for v in row):
                    issue(report, "上游待处理", "", "输入结果的“数据检查”仍存在未解决问题", name)
                    break
    except Exception:
        issue(report, "文件读取失败", "", "文件损坏或格式不受支持，请重新导出 Excel", name)
    finally:
        if wb:
            wb.close()
    errors = report["issues"][start:]
    item["status"] = "失败" if errors else "正常"
    item["detail"] = "；".join(e["detail"] for e in errors) if errors else f"识别 {len({p['code'] or p['name'] for p in item['people']})} 人"
    return item


def preflight(module, config):
    report = {"status": "passed", "target_month": config.get(f"{module}_target_month", ""), "files": [], "issues": []}
    value = str(report["target_month"])
    if not re.fullmatch(r"20\d{2}-(?:0[1-9]|1[0-2])", value):
        issue(report, "处理月份", "", "请先选择处理月份（YYYY-MM）")
        report["status"] = "needs_review"
        return report
    year, month = map(int, value.split("-"))
    report["days_in_month"] = calendar.monthrange(year, month)[1]
    for field, raw in config.items():
        if not field.startswith(module + "_"):
            continue
        for path in raw if isinstance(raw, list) else [raw]:
            if isinstance(path, str) and path.lower().endswith((".xlsx", ".xls")):
                report["files"].append(inspect_file(path, field, (year, month), report))
    if not report["files"]:
        issue(report, "输入为空", "", "请上传输入文件")
    names = defaultdict(set)
    for item in report["files"]:
        for person in item["people"]:
            names[person["name"]].add(person["code"])
    for name, codes in names.items():
        if "" in codes and len(codes - {""}) > 1:
            issue(report, "同名待确认", name, "多个工号同名，存在无工号来源，请补齐工号")
    report["status"] = "needs_review" if report["issues"] else "passed"
    return report


def write_sheets(wb, report):
    for name in ("文件检查", "数据检查"):
        if name in wb.sheetnames:
            continue
        ws = wb.create_sheet(name)
        if name == "文件检查":
            ws.append(["文件名", "类型", "状态", "说明"])
            for item in report["files"]:
                ws.append([item["file_name"], item["type"], item["status"], item["detail"]])
        else:
            ws.append(["检查项", "对象", "结果", "说明"])
            ws.append(["处理月份", report["target_month"], "通过", report.get("days_in_month", "")])
            ws.append(["检查状态", "", "通过" if report["status"] == "passed" else "待处理", "计算完成" if report["status"] == "passed" else "计算未完成"])
            for entry in report["issues"]:
                ws.append([entry["kind"], entry["person"], "待处理", entry["detail"]])
        ws.freeze_panes = "A2"
        for column, width in zip("ABCD", (32, 24, 14, 80)):
            ws.column_dimensions[column].width = width
        for row in ws.iter_rows(min_row=2):
            if row[2].value in {"待处理", "失败"}:
                for cell in row:
                    cell.fill = PatternFill("solid", fgColor="FFF1F0")


def finish(outputs, report, output_dir):
    for output in outputs:
        if output["path"].endswith(".xlsx"):
            wb = openpyxl.load_workbook(output["path"])
            write_sheets(wb, report)
            wb.save(output["path"])
            wb.close()
            check = openpyxl.load_workbook(output["path"], data_only=True, read_only=True)
            if not {"文件检查", "数据检查"}.issubset(check.sheetnames):
                check.close()
                raise ValueError("结果 Excel 检查页回读失败")
            check.close()
    if report.get("expected_people") is not None:
        for output in outputs:
            if not output["path"].endswith(".xlsx"):
                continue
            wb = openpyxl.load_workbook(output["path"])
            ws = wb["数据检查"]
            ws.append(["人员核对", "", "通过" if not report.get("missing_people") else "待处理",
                       f"应处理 {report['expected_people']} 人，识别 {report.get('recognized_people', 0)} 人，缺少 {report.get('missing_people', 0)} 人"])
            wb.save(output["path"])
            wb.close()
    meta = output_dir / "integrity_audit.json"
    meta.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    return outputs + [{"path": str(meta), "file_name": meta.name, "kind": "meta"}]


def blocked(report, output_dir):
    report["status"] = "needs_review"
    wb = openpyxl.Workbook()
    wb.remove(wb.active)
    write_sheets(wb, report)
    path = output_dir / "待处理检查报告.xlsx"
    wb.save(path)
    wb.close()
    return finish([{"path": str(path), "file_name": path.name, "kind": "audit"}], report, output_dir)


def run(module, config, output_dir, calculate):
    report = preflight(module, config)
    if report["status"] != "passed":
        return blocked(report, output_dir)
    try:
        if module == "parttime" and not config.get("parttime_roster"):
            raise ValueError("缺少钉钉人员名单，请从工具箱计算入口重新发起")
        outputs = calculate(config, output_dir)
    except ValueError as error:
        issue(report, "计算检查", "", str(error))
        return blocked(report, output_dir)
    for output in outputs:
        if output.get("kind") == "meta":
            data = json.loads(Path(output["path"]).read_text(encoding="utf-8"))
            if data.get("status") == "needs_review":
                issue(report, "数据待核对", "", "请查看结果中的数据检查页")
            subsidy = data.get("meta", {}).get("subsidy_audit", data.get("subsidy_audit", {}))
            if subsidy.get("missing_attendance_count"):
                issue(report, "考勤缺失", "", f"{subsidy['missing_attendance_count']} 人缺少考勤，请补充明细")
            for key in ("expected_people", "recognized_people", "missing_people", "fallback_count", "conflict_count", "blank_count"):
                if key in data:
                    report[key] = data[key]
    report["status"] = "needs_review" if report["issues"] else "passed"
    return finish(outputs, report, output_dir)
