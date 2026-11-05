"""Shared preflight and export audit for the five attendance workflows."""
from __future__ import annotations

import calendar
import json
import math
import re
from collections import defaultdict
from datetime import date, datetime
from pathlib import Path

import openpyxl
from openpyxl.styles import PatternFill
from excel_compat import load_workbook_compat

NAMES = {"姓名", "员工姓名", "人员姓名", "发起人姓名"}
CODES = {"工号", "员工工号", "员工编号", "发起人工号"}
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
    return re.sub(r"[\u200b-\u200d\ufeff]", "", str(value if value is not None else "")).strip()


def issue(report, kind, person, detail, file_name=""):
    report["issues"].append({"kind": kind, "person": person, "detail": detail, "file_name": file_name})


def safe_detail(error, config):
    detail = str(error)
    for raw in config.values():
        for path in raw if isinstance(raw, list) else [raw]:
            if isinstance(path, str) and Path(path).is_absolute():
                detail = detail.replace(path, Path(path).name)
    return detail


def inspect_file(path, field, target, report, original_name=None):
    name = original_name or Path(path).name
    item = {"file_name": name, "field": field, "type": "待识别", "status": "正常", "detail": "", "people": []}
    start = len(report["issues"])
    roster = field.endswith(("_active", "_resign", "_roster")) and field != "final_attendance_roster"
    approval = field in {"leave_src", "leave_reviewed", "leave_offsite_duration", "overtime_src", "final_leave", "final_overtime", "final_transfer"}
    calendar_file = field.endswith(("_calendar", "_default_schedule")) or field in {"leave_schedule", "subsidy_schedule", "final_schedule"}
    matrix = field.startswith("parttime_") or field.endswith(("_attendance", "_attendance_result", "_schedules"))
    wb = None
    file_identities = {}
    try:
        wb = load_workbook_compat(path, data_only=True)
        hints = set(months(name)) if not roster and not approval else set()
        matched_sheets = 0
        for ws in wb.worksheets:
            if ws.title in IGNORE_SHEETS:
                continue
            if not roster and not approval:
                hints.update(months(ws.title))
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
                # A month in the filename alone does not make a valid calendar.
                from calc_leave import load_schedule_context
                context = load_schedule_context(path, data_only=True)
                if (context["year"], context["month"]) != target:
                    issue(report, "月份冲突", "", "作息表日期与处理月份不一致", name)
                if not context.get("main_working_days"):
                    issue(report, "缺少日期", "", "作息表没有可识别的工作日期，请检查日期及公式缓存", name)
                matched_sheets += 1
                continue
            if header_row is None:
                continue
            matched_sheets += 1
            item["type"] = ("花名册" if roster else "审批明细" if approval else "排班表" if field.endswith("_schedules")
                            else "月度汇总" if field.endswith("_monthly") or "出勤天数" in headers else "考勤明细")
            name_col = next(i for i, h in enumerate(headers) if h in NAMES)
            code_col = next((i for i, h in enumerate(headers) if h in CODES), None)
            item.update(has_name=True, has_code=code_col is not None)
            date_cols = [i for i, h in enumerate(headers) if any(word in h for word in ("日期", "时间", "计算月份"))]
            day_cols = [i for i, h in enumerate(headers) if re.fullmatch(r"(?:[1-9]|[12]\d|3[01])(?:日|号)?", h) or (not approval and months(h))]
            # DingTalk two-row date headers put day numbers below 考勤结果.
            if matrix and not day_cols and header_row < ws.max_row:
                day_cols = [i for i in range(ws.max_column) if re.fullmatch(r"(?:[1-9]|[12]\d|3[01])(?:日|号)?", text(ws.cell(header_row + 1, i + 1).value))]
            if field.startswith("parttime_"):
                from calc_parttime_summary import _find_day_columns
                day_cols = [col - 1 for _, col in _find_day_columns(ws, header_row, allow_positional_fallback=False)]
            item["has_date"] = bool(date_cols or day_cols)
            if not roster and not approval:
                for header in headers:
                    hints.update(months(header))
            if not roster and not approval and not date_cols and not day_cols and "考勤结果" not in headers and not any("出勤" in h or "补贴" in h or "扣款" in h for h in headers):
                issue(report, "缺少日期", "", "未找到日期列、逐日表头或月度统计字段", name)
            if approval and not date_cols:
                issue(report, "缺少日期", "", "审批明细缺少开始、结束或业务日期列", name)
            seen_codes, seen_names = set(), defaultdict(list)
            for row_idx in range(header_row + 1, ws.max_row + 1):
                values = [ws.cell(row_idx, col).value for col in range(1, ws.max_column + 1)]
                employee = text(values[name_col])
                code = text(values[code_col]).upper() if code_col is not None else ""
                if employee in {"姓名", "日期", "星期", "合计", "总计"}:
                    continue
                if not employee:
                    is_date_header = row_idx == header_row + 1 and day_cols and all(
                        values[i] in (None, "") or re.fullmatch(r"(?:\d{1,2}(?:日|号)?|[一二三四五六日天]|星期[一二三四五六日天]|周[一二三四五六日天])", text(values[i]))
                        for i in day_cols)
                    if code or (not is_date_header and any(value not in (None, "") for value in values)):
                        issue(report, "空姓名", "", f"{ws.title} 第 {row_idx} 行姓名为空", name)
                    continue
                if employee.startswith("【示例】"):
                    issue(report, "模板示例", employee, "请删除示例行或替换为真实员工信息", name)
                # Daily long tables legitimately repeat one employee on different dates.
                identity = (code, tuple(text(values[i]) for i in date_cols)) if date_cols and not day_cols and not roster else code
                if code and identity in seen_codes and not approval:
                    issue(report, "重复工号", employee, f"{ws.title} 第 {row_idx} 行工号 {code} 重复", name)
                if code:
                    seen_codes.add(identity)
                    previous = file_identities.get(identity)
                    if previous and previous != ws.title and not approval:
                        issue(report, "重复工号", employee, f"工号 {code} 同时出现在 {previous} 和 {ws.title}", name)
                    file_identities[identity] = ws.title
                seen_names[employee].append(code)
                item["people"].append({"name": employee, "code": code, "sheet": ws.title, "row": row_idx})
                if matrix and day_cols and all(values[i] in (None, "") for i in day_cols):
                    issue(report, "整月空白", f"{code} {employee}".strip(), "没有逐日考勤记录", name)
                if not roster and not approval:
                    for i in date_cols:
                        hints.update(months(values[i]))
                if approval and date_cols and not any(values[i] not in (None, "") for i in date_cols):
                    issue(report, "缺少日期", employee, f"{ws.title} 第 {row_idx} 行没有日期", name)
                if "计算月份" in headers:
                    computed_month = months(values[headers.index("计算月份")])
                    if any((year or target[0], month) != target for year, month in computed_month):
                        issue(report, "月份冲突", employee, "上游结果的计算月份与处理月份不一致", name)
            for employee, codes in seen_names.items():
                if len(codes) > 1 and "" in codes and not approval and (day_cols or roster):
                    issue(report, "同名待确认", employee, "同名记录缺少工号，请补齐工号后重试", name)
        if not matched_sheets:
            issue(report, "未识别文件", "", "未找到姓名列或受支持的文件结构", name)
        if field.startswith("parttime_") and not calendar_file and matched_sheets > 1:
            issue(report, "文件结构", "", "包含多个人员数据工作表，请拆分上传以逐表检查和计算", name)
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
    item["detail"] = "；".join(e["detail"] for e in errors) if errors else (
        "作息日期已识别" if calendar_file else f"识别 {len({p['code'] or p['name'] for p in item['people']})} 人" + ("；无工号列，按唯一姓名匹配" if not item.get("has_code") else ""))
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
    for field, raw in list(config.items()):
        if not field.startswith(module + "_"):
            continue
        for path in raw if isinstance(raw, list) else [raw]:
            if isinstance(path, str) and path.lower().endswith((".xlsx", ".xls")):
                report["files"].append(inspect_file(path, field, (year, month), report, config.get("_original_names", {}).get(path)))
    if not report["files"]:
        issue(report, "输入为空", "", "请上传输入文件")
    names = defaultdict(set)
    codes_to_names = defaultdict(set)
    for item in report["files"]:
        for person in item["people"]:
            names[person["name"]].add(person["code"])
            if person["code"]:
                codes_to_names[person["code"]].add(person["name"])
    for code, employees in codes_to_names.items():
        if len(employees) > 1:
            issue(report, "工号身份冲突", code, "同一工号对应不同姓名：" + "、".join(sorted(employees)))
    for name, codes in names.items():
        if "" in codes and len(codes - {""}) > 1:
            issue(report, "同名待确认", name, "多个工号同名，存在无工号来源，请补齐工号")
    if module == "parttime" and not report["issues"]:
        from calc_parttime_summary import prepare_parttime_data, DEFAULT_SPECIAL_DEFAULT_NAMES, _split_person_key, _person_code
        def paths(field):
            raw = config.get(field) or []
            return raw if isinstance(raw, list) else [raw]
        special = config.get("part_special_names", DEFAULT_SPECIAL_DEFAULT_NAMES)
        if isinstance(special, str):
            special = [name.strip() for name in re.split(r"[,，、;；\n]+", special) if name.strip()]
        try:
            prepared = prepare_parttime_data(paths("parttime_attendance_detail"), paths("parttime_monthly"),
                paths("parttime_schedules"), year, month, special, config.get("parttime_roster"),
                config.get("final_auto_resign_json"), config.get("parttime_conflict_policy", "block"))
            config["_parttime_prepared"] = prepared
            for filename, kind, source in prepared["source_records"]:
                if not source:
                    issue(report, "未识别人员", "", "未识别到可处理的兼职或实习人员，请核对人员类型和工号", filename)
                for key, day_map in source.items():
                    if not day_map or all(entry.get("value") is None for entry in day_map.values()):
                        issue(report, "整月空白", f"{_person_code(key)} {_split_person_key(key)[0]}".strip(), "整月没有可计算的记录，请补齐或确认源记录", filename)
            if config.get("parttime_roster"):
                report["expected_people"] = len(prepared["expected_people"])
                report["missing_people"] = len(prepared["missing_people"])
                report["recognized_people"] = report["expected_people"] - report["missing_people"]
                for key, label in (("parttime_roster", "组织花名册快照"), ("final_auto_resign_json", "目标月离职名单")):
                    if config.get(key):
                        report["files"].append({"file_name": Path(config[key]).name, "field": key, "type": label,
                            "status": "正常", "detail": f"用于人员基准核对；目标月共 {report['expected_people']} 人", "people": []})
                for person in prepared["missing_people"]:
                    issue(report, "缺少人员", person, "花名册存在，但输入文件没有匹配记录")
                for key in prepared["unexpected_people"]:
                    issue(report, "花名册未匹配", f"{_person_code(key)} {_split_person_key(key)[0]}".strip(), "输入人员未匹配到目标月花名册，请核对工号、人员类型和入离职日期")
            for key in prepared["unmatched_schedules"]:
                issue(report, "排班未匹配", f"{_person_code(key)} {_split_person_key(key)[0]}".strip(), "排班未匹配到考勤明细或月度记录")
            report["conflicts"] = prepared["conflicts"]
            for conflict in prepared["conflicts"]:
                if not conflict["resolved"]:
                    issue(report, "来源冲突", conflict["name"], f"{month}月{conflict['day']}日：" + "；".join(
                        f"{value['source']}={value['value']}" for value in conflict["values"]))
        except ValueError as error:
            detail = safe_detail(error, config)
            issue(report, "同名待确认" if "同名员工" in detail else "输入数据", detail if "同名员工" in detail else "", detail)
    summarize(report)
    return report


def summarize(report):
    for item in report["files"]:
        errors = [entry["detail"] for entry in report["issues"] if entry.get("file_name") == item["file_name"]]
        if errors:
            item.update(status="失败", detail="；".join(dict.fromkeys(errors)))
    report["unrecognized_files"] = sum(item["status"] == "失败" for item in report["files"])
    report["same_name_groups"] = len({item["person"] for item in report["issues"] if item["kind"] == "同名待确认"})
    if "conflicts" in report:
        report["conflict_count"] = sum(not item.get("resolved") for item in report["conflicts"])
        report["resolved_conflict_count"] = sum(bool(item.get("resolved")) for item in report["conflicts"])
    report["status"] = "needs_review" if report["issues"] else "passed"


def write_sheets(wb, report):
    previous = []
    if "数据检查" in wb.sheetnames:
        previous = [list(row) for row in wb["数据检查"].iter_rows(min_row=2, values_only=True)
                    if row and row[0] not in {"处理月份", "检查状态", "人员核对", "合计回读"}]
    for name in ("文件检查", "数据检查"):
        if name in wb.sheetnames:
            del wb[name]
        ws = wb.create_sheet(name)
        if name == "文件检查":
            ws.append(["文件名", "类型", "状态", "说明"])
            for item in report["files"]:
                ws.append([item["file_name"], item["type"], item["status"], item["detail"]])
        else:
            ws.append(["检查项", "对象", "结果", "说明"])
            ws.append(["处理月份", report["target_month"], "通过" if report.get("days_in_month") else "待处理", report.get("days_in_month", "")])
            completed = report["status"] == "passed" and report.get("totals_verified")
            ws.append(["检查状态", "", "通过" if completed else "待处理", "计算完成" if completed else "计算未完成"])
            if report.get("expected_people") is not None:
                ws.append(["人员核对", "", "通过" if not report.get("missing_people") else "待处理",
                           f"应处理 {report['expected_people']} 人，识别 {report.get('recognized_people', 0)} 人，缺少 {report.get('missing_people', 0)} 人"])
            ws.append(["合计回读", "", "通过" if report.get("totals_verified") else "待处理",
                       "合计已写入数字并回读验证" if report.get("totals_verified") else "检查通过后执行计算与回读"])
            for row in previous:
                ws.append(row)
            for entry in report["issues"]:
                ws.append([entry["kind"], entry["person"], "待处理", (entry.get("file_name", "") + " " + entry["detail"]).strip()])
            if not any(row[0] == "来源冲突" for row in previous):
                for conflict in report.get("conflicts", []):
                    detail = "；".join(f"{value['source']}={value['value']}" for value in conflict["values"])
                    if conflict.get("resolved"):
                        detail += f"；按 {conflict['policy']} 采用 {conflict['selected_source']}={conflict['selected_value']}"
                    ws.append(["来源冲突", f"{conflict['name']} / {conflict['day']}日", "已处理" if conflict.get("resolved") else "待处理", detail])
        ws.freeze_panes = "A2"
        for column, width in zip("ABCD", (32, 24, 14, 80)):
            ws.column_dimensions[column].width = width
        for row in ws.iter_rows(min_row=2):
            if row[2].value in {"待处理", "失败"}:
                for cell in row:
                    cell.fill = PatternFill("solid", fgColor="FFF1F0")


def verify_totals(path):
    """Check saved numeric totals, including final payroll, using Excel value mode."""
    wb = openpyxl.load_workbook(path, read_only=True, data_only=True)
    try:
        for ws in wb.worksheets:
            if ws.title in IGNORE_SHEETS:
                continue
            for row in ws.iter_rows(min_row=1, max_row=min(ws.max_row, 15)):
                totals = [cell.column for cell in row if text(cell.value) in {
                    "出勤天数", "真实记录天数", "系统补填天数", "实际出勤天数", "计薪出勤天数", "合计"}]
                if not totals:
                    continue
                for values in ws.iter_rows(min_row=row[0].row + 1, values_only=True):
                    if not any(value is not None for value in values):
                        continue
                    for col in totals:
                        value = values[col - 1]
                        if value is None or isinstance(value, bool) or not isinstance(value, (int, float)) or not math.isfinite(value):
                            raise ValueError(f"{ws.title} 的合计回读不是有效数字")
                break
    finally:
        wb.close()


def finish(outputs, report, output_dir):
    exports = [output for output in outputs if output.get("kind") != "audit" and output["path"].endswith(".xlsx")]
    if exports:
        try:
            for output in exports:
                verify_totals(output["path"])
            report["totals_verified"] = True
        except ValueError as error:
            report["totals_verified"] = False
            issue(report, "合计回读", "", str(error))
            return blocked(report, output_dir)
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
            if output in exports:
                verify_totals(output["path"])
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
    meta = output_dir / "integrity_audit.json"
    meta.write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding="utf-8")
    return [{"path": str(path), "file_name": path.name, "kind": "audit"},
            {"path": str(meta), "file_name": meta.name, "kind": "meta"}]


def run(module, config, output_dir, calculate):
    report = preflight(module, config)
    if report["status"] != "passed":
        return blocked(report, output_dir)
    try:
        if module == "parttime" and not config.get("parttime_roster"):
            raise ValueError("缺少钉钉人员名单，请从工具箱计算入口重新发起")
        outputs = calculate(config, output_dir)
    except ValueError as error:
        issue(report, "计算检查", "", safe_detail(error, config))
        return blocked(report, output_dir)
    for output in outputs:
        if output.get("kind") == "meta":
            data = json.loads(Path(output["path"]).read_text(encoding="utf-8"))
            if data.get("status") == "needs_review":
                issue(report, "数据待核对", "", "请查看结果中的数据检查页")
            subsidy = data.get("meta", {}).get("subsidy_audit", data.get("subsidy_audit", {}))
            if subsidy.get("missing_attendance_count"):
                issue(report, "考勤缺失", "", f"{subsidy['missing_attendance_count']} 人缺少考勤，请补充明细")
            for key in ("expected_people", "recognized_people", "missing_people", "fallback_count", "conflict_count", "resolved_conflict_count", "blank_count", "conflicts"):
                if key in data:
                    report[key] = data[key]
    summarize(report)
    return finish(outputs, report, output_dir)
