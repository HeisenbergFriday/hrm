#!/usr/bin/env python3
"""
考勤数据处理 CLI 入口
由 Go 后端通过 subprocess 调用，不修改原有业务模块代码。

用法:
    python cli.py leave       --input <file> --schedule <file> --output-dir <dir>
    python cli.py overtime    --input <file> [--schedule <file>] [--attendance <file>] [--roster <file>] [--config <file>] --output-dir <dir>
    python cli.py subsidy     --source <file> --attendance <file> --schedule <file> [--signin <file>] [--result <file>] --output-dir <dir>
    python cli.py final       --roster <file> --schedule <file> --leave <file> --overtime <file> --subsidy <file> [--resigned <file>] [--transfer <file>] --output-dir <dir>
    python cli.py parttime    --output <file> [--default-schedule <file>] [--monthly-summary <file>...] [--schedule <file>...]

结果以 JSON 输出到 stdout: {"status":"ok","output":"<path>","message":"..."}
错误以 JSON 输出到 stdout: {"status":"error","error":"<message>"}
"""
from __future__ import annotations

import argparse
import json
import os
import sys
import traceback

# ── 子模块路径注入 ──
_BASE = os.path.dirname(os.path.abspath(__file__))
for _sub in ("leave", "overtime", "subsidy", "finally", "parttime"):
    _p = os.path.join(_BASE, _sub)
    if _p not in sys.path:
        sys.path.insert(0, _p)


def _ok(output: str, message: str = "") -> None:
    print(json.dumps({"status": "ok", "output": output, "message": message}, ensure_ascii=False))


def _fail(error: str) -> None:
    print(json.dumps({"status": "error", "error": error}, ensure_ascii=False))
    sys.exit(1)


# ═══════════════════════════════════════════════════════════════════════════════
# 请假明细
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_leave(args: argparse.Namespace) -> None:
    import calc_leave

    if not os.path.isfile(args.input):
        _fail(f"输入文件不存在: {args.input}")
    if not os.path.isfile(args.schedule):
        _fail(f"作息表文件不存在: {args.schedule}")

    os.makedirs(args.output_dir, exist_ok=True)
    out_file = os.path.join(args.output_dir, "leave_detail.xlsx")

    schedule_ctx = calc_leave.load_schedule_context(args.schedule)
    src_rows = calc_leave.clean_export(args.input)
    calc_leave.process(src_rows, out_file, schedule_ctx)

    _ok(out_file, f"处理完成，共 {len(src_rows) - 1} 条记录")


# ═══════════════════════════════════════════════════════════════════════════════
# 加班明细
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_overtime(args: argparse.Namespace) -> None:
    import fill_overtime_fields as ot
    from rules_engine import load_config_or_default

    if not os.path.isfile(args.input):
        _fail(f"输入文件不存在: {args.input}")

    os.makedirs(args.output_dir, exist_ok=True)
    out_file = os.path.join(args.output_dir, "overtime_detail_filled.xlsx")

    # 加载规则配置
    rules_config = load_config_or_default(args.config)

    # 清洗输入
    src_rows = ot.clean_export_overtime(args.input)

    # 可选：加载排班表
    schedule_map = {}
    if args.schedule and os.path.isfile(args.schedule):
        schedule_map, _ = ot.load_schedule_if_available(args.schedule, rules_config=rules_config)

    # 可选：加载考勤打卡明细
    attendance_missing_map = {}
    if args.attendance and os.path.isfile(args.attendance):
        attendance_missing_map, _ = ot.load_attendance_if_available(args.attendance)

    # 可选：加载花名册（部门修正）
    employee_dept_map = {}
    if args.roster and os.path.isfile(args.roster):
        employee_dept_map = ot.parse_employee_department_map(args.roster)

    # 处理
    ot.process_overtime(
        src_rows, out_file, schedule_map, attendance_missing_map,
        rules_config=rules_config,
        employee_department_map=employee_dept_map if employee_dept_map else None,
    )

    _ok(out_file, f"处理完成，共 {len(src_rows) - 1} 条记录")


# ═══════════════════════════════════════════════════════════════════════════════
# 补贴扣款核对
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_subsidy(args: argparse.Namespace) -> None:
    import calc_subsidy_deduction as sub
    from datetime import date

    if not os.path.isfile(args.source):
        _fail(f"补贴扣款源文件不存在: {args.source}")
    if not os.path.isfile(args.attendance):
        _fail(f"考勤文件不存在: {args.attendance}")
    if not os.path.isfile(args.schedule):
        _fail(f"作息表文件不存在: {args.schedule}")

    os.makedirs(args.output_dir, exist_ok=True)
    out_file = os.path.join(args.output_dir, "subsidy_checked.xlsx")
    mismatch_file = os.path.join(args.output_dir, "subsidy_mismatch_report.xlsx")

    # 解析作息表获取年月
    year, month, _ = sub.resolve_schedule_period(args.schedule)

    # 解析补贴扣款源表
    source_records = sub.parse_source_table(args.source)

    # 解析考勤数据
    employees = sub.parse_attendance(
        args.attendance, year=year, month=month, period_source="作息表"
    )

    # 解析签到表（可选）
    activity_days: dict[date, set] = {}
    if args.signin and os.path.isfile(args.signin):
        activity_days = sub.parse_activity_checkin(args.signin)

    # 法定节假日
    legal_holidays = sub.load_statutory_holidays_from_schedule(args.schedule, year, month)

    # 考勤结果表（可选，用于排除已补流程）
    approved_dates_by_name: dict[str, set[date]] = {}
    if args.result and os.path.isfile(args.result):
        approved_dates_by_name = sub.parse_attendance_result(args.result, year, month)

    # 写出核对表
    mismatches, missing_att = sub.write_output(
        args.source, source_records, employees, activity_days, out_file,
        approved_dates_by_name=approved_dates_by_name,
        legal_holidays=legal_holidays,
    )

    # 写出差异清单
    if mismatches:
        sub.write_mismatch_report(mismatches, mismatch_file)
        _ok(out_file, f"核对完成，发现 {len(mismatches)} 处差异，{missing_att} 人缺少考勤数据")
    else:
        _ok(out_file, f"核对完成，无差异，{missing_att} 人缺少考勤数据")


# ═══════════════════════════════════════════════════════════════════════════════
# 最终表生成
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_final(args: argparse.Namespace) -> None:
    import calc_leave
    import calc_finally as fin

    if not os.path.isfile(args.roster):
        _fail(f"花名册文件不存在: {args.roster}")
    if not os.path.isfile(args.schedule):
        _fail(f"作息表文件不存在: {args.schedule}")
    if not os.path.isfile(args.leave):
        _fail(f"请假明细表不存在: {args.leave}")
    if not os.path.isfile(args.overtime):
        _fail(f"加班明细表不存在: {args.overtime}")
    if not os.path.isfile(args.subsidy):
        _fail(f"补贴扣款表不存在: {args.subsidy}")

    os.makedirs(args.output_dir, exist_ok=True)
    out_file = os.path.join(args.output_dir, "final_attendance.xlsx")

    # 解析花名册
    employees = fin.parse_roster(args.roster, args.resigned)

    # 解析异动
    transfer_map = {}
    if args.transfer and os.path.isfile(args.transfer):
        transfer_map = fin.parse_transfer(args.transfer)

    # 解析作息表
    schedule_ctx = fin.parse_schedule(args.schedule)

    # 解析请假明细
    leave_map = fin.parse_leave_summary(args.leave, schedule_ctx)
    leave_day_details = fin.parse_leave_day_details(args.leave, schedule_ctx)

    # 解析加班明细
    overtime_map = fin.parse_overtime_summary(
        args.overtime, schedule_ctx["year"], schedule_ctx["month"],
        employees, schedule_ctx
    )

    # 解析补贴扣款（旷工天数）
    absent_by_no, absent_by_name = fin.parse_subsidy_absent(args.subsidy)
    absent_day_details = fin.parse_subsidy_absent_day_details(
        args.subsidy, schedule_ctx["year"], schedule_ctx["month"]
    )

    # 生成最终表
    fin.generate(
        employees, transfer_map, schedule_ctx, leave_map, overtime_map,
        absent_by_no, absent_by_name, out_file, leave_day_details,
        absent_day_details=absent_day_details,
    )

    _ok(out_file, f"生成完成，共 {len(employees)} 名员工")


# ═══════════════════════════════════════════════════════════════════════════════
# 兼职汇总
# ═══════════════════════════════════════════════════════════════════════════════

def cmd_parttime(args: argparse.Namespace) -> None:
    import calc_parttime_summary as part

    os.makedirs(args.output_dir, exist_ok=True)
    out_file = os.path.join(args.output_dir, "parttime_summary.xlsx")

    result = part.generate_parttime_summary(
        output_path=out_file,
        attendance_detail_path=args.attendance_detail,
        monthly_summary_paths=args.monthly_summary or [],
        schedule_paths=args.schedule or [],
        default_schedule_path=args.default_schedule,
    )

    _ok(out_file, f"生成完成，共 {result.get('employee_count', '?')} 名员工")


# ═══════════════════════════════════════════════════════════════════════════════
# CLI 注册
# ═══════════════════════════════════════════════════════════════════════════════

def build_parser() -> argparse.ArgumentParser:
    parser = argparse.ArgumentParser(description="考勤数据处理 CLI")
    sub = parser.add_subparsers(dest="command", required=True)

    # leave
    p_leave = sub.add_parser("leave", help="请假明细计算")
    p_leave.add_argument("--input", required=True, help="请假系统导出 xlsx")
    p_leave.add_argument("--schedule", required=True, help="作息表 xlsx")
    p_leave.add_argument("--output-dir", required=True, help="输出目录")
    p_leave.set_defaults(func=cmd_leave)

    # overtime
    p_ot = sub.add_parser("overtime", help="加班明细处理")
    p_ot.add_argument("--input", required=True, help="加班系统导出 xlsx")
    p_ot.add_argument("--schedule", help="排班表 xlsx（可选）")
    p_ot.add_argument("--attendance", help="考勤打卡明细 xlsx（可选）")
    p_ot.add_argument("--roster", help="花名册 xlsx（可选，用于部门修正）")
    p_ot.add_argument("--config", help="加班规则配置 xlsx（可选）")
    p_ot.add_argument("--output-dir", required=True, help="输出目录")
    p_ot.set_defaults(func=cmd_overtime)

    # subsidy
    p_sub = sub.add_parser("subsidy", help="补贴扣款核对")
    p_sub.add_argument("--source", required=True, help="补贴扣款源表 xlsx")
    p_sub.add_argument("--attendance", required=True, help="考勤月度汇总 xlsx")
    p_sub.add_argument("--schedule", required=True, help="作息表 xlsx")
    p_sub.add_argument("--signin", help="签到表 xlsx（可选）")
    p_sub.add_argument("--result", help="考勤结果表 xlsx（可选）")
    p_sub.add_argument("--output-dir", required=True, help="输出目录")
    p_sub.set_defaults(func=cmd_subsidy)

    # final
    p_fin = sub.add_parser("final", help="最终表生成")
    p_fin.add_argument("--roster", required=True, help="在职花名册 xlsx")
    p_fin.add_argument("--resigned", help="离职花名册 xlsx（可选）")
    p_fin.add_argument("--transfer", help="异动流程表 xlsx（可选）")
    p_fin.add_argument("--schedule", required=True, help="作息表 xlsx")
    p_fin.add_argument("--leave", required=True, help="请假明细表 xlsx")
    p_fin.add_argument("--overtime", required=True, help="加班明细_回填 xlsx")
    p_fin.add_argument("--subsidy", required=True, help="补贴扣款表_核对 xlsx")
    p_fin.add_argument("--output-dir", required=True, help="输出目录")
    p_fin.set_defaults(func=cmd_final)

    # parttime
    p_pt = sub.add_parser("parttime", help="兼职汇总")
    p_pt.add_argument("--default-schedule", help="默认作息表 xlsx（可选）")
    p_pt.add_argument("--attendance-detail", help="考勤明细 xlsx（可选）")
    p_pt.add_argument("--monthly-summary", nargs="*", help="月度汇总 xlsx（可选，可多个）")
    p_pt.add_argument("--schedule", nargs="*", help="排班表 xlsx（可选，可多个）")
    p_pt.add_argument("--output-dir", required=True, help="输出目录")
    p_pt.set_defaults(func=cmd_parttime)

    return parser


def main() -> None:
    parser = build_parser()
    args = parser.parse_args()
    try:
        args.func(args)
    except Exception as e:
        _fail(f"{type(e).__name__}: {e}")


if __name__ == "__main__":
    main()
