# AI Development Spec: Annual Leave and Compensatory Time

## 1. Document Goal

This document is for AI-assisted development of the following three requirements in the `People Ops` project:

1. Optimize annual leave eligibility calculation.
2. Optimize annual leave grant rules.
3. Require overtime approval and convert approved overtime attendance duration into compensatory time.

This spec is implementation-oriented. The AI developer should follow this document to complete database design, backend services, scheduled jobs, APIs, and related tests while minimizing DingTalk API calls.

## 2. Core Constraint: Minimize DingTalk API Calls

The system must use a "sync once, calculate locally" strategy.

Rules:

1. Do not call DingTalk APIs during page queries, leave calculation, leave grant calculation, overtime matching, or compensatory time balance queries.
2. DingTalk APIs are allowed only in scheduled or manual sync jobs.
3. Attendance data and approval data must be stored locally first, then all business logic must run on local tables.
4. Employee profile fields such as `EntryDate` and `ProbationEndDate` must be treated as the primary source for annual leave calculation.
5. If a required field is missing from local data, prefer adding or maintaining it in local profile tables instead of querying DingTalk in real time.

## 3. Existing Relevant Code

Existing local models and services already provide a good base:

- Employee profile: `internal/database/models.go`, `EmployeeProfile`
- Attendance records: `internal/database/models.go`, `Attendance`
- Approval records: `internal/database/models.go`, `Approval`
- Employee service: `internal/service/employee_service.go`
- Attendance service: `internal/service/attendance_service.go`
- Approval service: `internal/service/approval_service.go`
- Employee repository: `internal/repository/employee_repository.go`
- Attendance repository: `internal/repository/attendance_repository.go`
- Approval repository: `internal/repository/approval_repository.go`

The current system already caches DingTalk-origin data locally. The new implementation should extend this local-first architecture instead of adding real-time external dependency.

## 4. Business Scope

### 4.1 Requirement A: Annual Leave Eligibility Calculation

Business example:

- If an employee is not confirmed in Q2 but becomes confirmed in Q3, Q2 should still be counted according to the new rule.

This means annual leave eligibility is not simply "current quarter confirmed or not". It requires retrospective quarter eligibility recalculation after confirmation.

### 4.2 Requirement B: Annual Leave Grant Rule

Business rule:

1. Annual leave is granted quarterly.
2. Grant days depend on working years.

This requires a local rule engine and a grant ledger.

### 4.3 Requirement C: Overtime Approval + Attendance-Based Compensatory Time

Business rule:

1. Overtime must have an approval request.
2. Only approved overtime can be considered.
3. The final compensatory time added must be based on overtime attendance duration.

This requires local matching of approval records and attendance records.

## 5. Recommended Delivery Order

The AI developer should implement in this order:

1. Annual leave eligibility engine.
2. Annual leave grant engine and grant ledger.
3. Overtime approval matching and compensatory time ledger.

Reason:

- Requirement B depends on the results of A.
- Requirement C spans approval, attendance, and balance ledgers, so it has the largest integration surface.

## 6. Key Business Assumptions

Unless the user overrides them later, the AI developer should use the following default assumptions.

### 6.1 Annual Leave Eligibility Assumptions

1. Calculation unit is quarter.
2. Eligibility is based on employee local profile data.
3. `EntryDate` is the employee onboarding date.
4. `ProbationEndDate` is treated as the confirmation date.
5. If confirmation happens in Q3, and the rule says previous quarter should be counted, the system must support retroactive eligibility for Q2.
6. Eligibility results must be persisted, not calculated on every query.

### 6.2 Annual Leave Grant Assumptions

1. Grant happens once per quarter.
2. Grant days are determined by working years.
3. Working years should be calculated locally based on a configurable reference date.
4. Retroactive grants are allowed when eligibility changes after confirmation or profile correction.
5. Every grant and re-grant must leave an audit trail.

### 6.3 Overtime Assumptions

1. Overtime must have a matched approval record.
2. Only approved approvals are valid.
3. Overtime duration comes from local attendance records, not directly from approval form duration.
4. Approval data is used as qualification evidence, while attendance data is used as the final duration source.
5. Duplicate matching and duplicate balance credit must be prevented.
6. Approval cancellation or rollback must be traceable and reversible.

## 7. Data Model Design

The AI developer should add the following local tables or equivalent GORM models.

### 7.1 `leave_rule_configs`

Purpose:

- Store annual leave rules without hard-coding them into service logic.

Suggested fields:

- `id`
- `rule_type` - `eligibility` / `grant`
- `rule_key`
- `rule_name`
- `rule_value_json`
- `status`
- `effective_from`
- `effective_to`
- `created_at`
- `updated_at`

Examples of `rule_value_json`:

- quarter retroactive confirmation policy
- working years to annual leave days mapping
- quarterly split strategy

### 7.2 `annual_leave_eligibility`

Purpose:

- Persist quarterly eligibility results by employee and year.

Suggested fields:

- `id`
- `user_id`
- `year`
- `quarter`
- `entry_date`
- `confirmation_date`
- `is_eligible`
- `eligible_start_date`
- `eligible_end_date`
- `retroactive_source_quarter`
- `calc_version`
- `calc_reason`
- `created_at`
- `updated_at`

Constraints:

- Unique index on `user_id + year + quarter`

### 7.3 `annual_leave_grants`

Purpose:

- Persist quarterly grant results and balance movements.

Suggested fields:

- `id`
- `user_id`
- `year`
- `quarter`
- `working_years`
- `base_days`
- `granted_days`
- `retroactive_days`
- `used_days`
- `remaining_days`
- `grant_type` - `normal` / `retroactive` / `adjustment`
- `source_eligibility_id`
- `remark`
- `created_at`
- `updated_at`

Constraints:

- Index on `user_id + year`

### 7.4 `overtime_rule_configs`

Purpose:

- Store overtime qualification and conversion rules.

Suggested fields:

- `id`
- `rule_key`
- `rule_name`
- `rule_value_json`
- `status`
- `effective_from`
- `effective_to`
- `created_at`
- `updated_at`

Examples:

- minimum overtime threshold
- meal break deduction policy
- round-down or round-up policy
- compensatory conversion ratio
- matching time window

### 7.5 `overtime_match_results`

Purpose:

- Store local matching result between approval and attendance.

Suggested fields:

- `id`
- `user_id`
- `approval_id`
- `approval_process_id`
- `approval_status`
- `approval_start_time`
- `approval_end_time`
- `attendance_start_time`
- `attendance_end_time`
- `matched_minutes`
- `qualified_minutes`
- `match_status` - `matched` / `partial` / `unmatched` / `rolled_back`
- `match_reason`
- `calc_version`
- `created_at`
- `updated_at`

Constraints:

- Unique index on `approval_id`

### 7.6 `compensatory_leave_ledger`

Purpose:

- Persist credited compensatory time and balance consumption.

Suggested fields:

- `id`
- `user_id`
- `source_type` - `overtime`
- `source_match_id`
- `credit_minutes`
- `debit_minutes`
- `balance_minutes`
- `ledger_type` - `credit` / `debit` / `rollback` / `adjustment`
- `effective_date`
- `expire_date`
- `remark`
- `created_at`
- `updated_at`

Constraints:

- Index on `user_id + effective_date`

## 8. Service Layer Design

The AI developer should add new services instead of overloading existing attendance and approval services with too much logic.

### 8.1 `AnnualLeaveService`

Responsibilities:

1. Load employee profile data from local database.
2. Load active leave rules from `leave_rule_configs`.
3. Calculate quarterly eligibility for a given employee and year.
4. Persist results to `annual_leave_eligibility`.
5. Recalculate when employee profile fields change.

Suggested methods:

- `RecalculateEligibility(userID string, year int) error`
- `RecalculateEligibilityBatch(year int, userIDs []string) error`
- `GetEligibility(userID string, year int) ([]EligibilityResult, error)`

### 8.2 `AnnualLeaveGrantService`

Responsibilities:

1. Load eligibility results.
2. Determine working years by configured rule.
3. Map working years to annual leave days.
4. Split or allocate days by quarter.
5. Persist grant ledger records.
6. Support retroactive grant.

Suggested methods:

- `GrantQuarter(year int, quarter int) error`
- `GrantForUser(userID string, year int, quarter int) error`
- `RegrantForEligibilityChange(userID string, year int) error`
- `GetGrantLedger(userID string, year int) ([]GrantRecord, error)`

### 8.3 `OvertimeMatchingService`

Responsibilities:

1. Load locally synced approval data.
2. Identify which approval records are overtime approvals.
3. Load locally synced attendance records.
4. Match approval windows against attendance windows.
5. Compute qualified overtime minutes.
6. Persist match results.
7. Generate compensatory time ledger credits.

Suggested methods:

- `MatchApprovedOvertime(startDate, endDate string) error`
- `MatchApproval(approvalID uint) error`
- `RollbackApprovalMatch(approvalID uint) error`
- `GetMatchResults(userID string, startDate, endDate string) ([]MatchResult, error)`

### 8.4 `CompensatoryLeaveService`

Responsibilities:

1. Read compensatory leave ledger.
2. Return current balance.
3. Support future debit logic if leave consumption is added later.

Suggested methods:

- `GetBalance(userID string) (BalanceResult, error)`
- `CreditFromOvertime(matchID uint) error`
- `RollbackCredit(matchID uint) error`

## 9. Repository Layer Design

The AI developer should add repositories for the new models:

- `LeaveRuleConfigRepository`
- `AnnualLeaveEligibilityRepository`
- `AnnualLeaveGrantRepository`
- `OvertimeRuleConfigRepository`
- `OvertimeMatchResultRepository`
- `CompensatoryLeaveLedgerRepository`

Repository rules:

1. Keep repositories focused on persistence only.
2. Put business rules into service layer, not repository layer.
3. Provide idempotent lookup methods for rerun safety.

## 10. API Design

The AI developer should provide local-data APIs only. Do not call DingTalk from handlers.

### 10.1 Annual Leave Eligibility APIs

- `GET /api/v1/leave/eligibility?user_id=&year=`
- `POST /api/v1/leave/eligibility/recalculate`

Request example:

```json
{
  "user_id": "xxx",
  "year": 2026
}
```

### 10.2 Annual Leave Grant APIs

- `GET /api/v1/leave/grants?user_id=&year=`
- `POST /api/v1/leave/grants/run-quarter`
- `POST /api/v1/leave/grants/regrant`

Request example:

```json
{
  "year": 2026,
  "quarter": 3
}
```

### 10.3 Overtime and Compensatory APIs

- `GET /api/v1/overtime/matches?user_id=&start_date=&end_date=`
- `POST /api/v1/overtime/matches/run`
- `GET /api/v1/comp-time/balance?user_id=`

Request example:

```json
{
  "start_date": "2026-04-01",
  "end_date": "2026-04-30"
}
```

## 11. Scheduled Jobs

To minimize DingTalk API calls, business processing must be separated from sync processing.

### 11.1 Sync Jobs

Keep or add scheduled jobs for:

1. Approval sync job
2. Attendance sync job

Rules:

1. Sync jobs fetch DingTalk data and write to local database.
2. They must support incremental sync by time range.
3. They must not perform business grant or compensatory calculations directly.

### 11.2 Business Jobs

Add local-only jobs for:

1. Quarterly annual leave grant job
2. Eligibility recalculation job
3. Overtime approval matching job
4. Compensatory balance correction job if needed

Rules:

1. Business jobs read only local database.
2. Business jobs must be rerunnable and idempotent.
3. Repeated execution must not create duplicate grants or duplicate credits.

## 12. Overtime Matching Rules

The AI developer should implement the following default matching strategy unless changed later.

1. Only approvals with approved status are eligible.
2. Overtime approval type should be identified from locally stored approval template ID, category, or extension fields.
3. Match the employee by `ApplicantID` with `Attendance.UserID`.
4. Match attendance records inside the approved overtime time window.
5. Compute actual overtime duration from attendance check-in and check-out records.
6. If attendance duration is shorter than approved duration, use attendance duration.
7. If no valid attendance exists, no compensatory time is credited.
8. Each approval can produce at most one effective match result.
9. Re-running the job must update or skip existing matched records safely.

## 13. Annual Leave Grant Rules

The AI developer should implement a configurable rule engine, but use this default logic first.

### 13.1 Eligibility Logic

1. Evaluate each employee by year and quarter.
2. Use local `EntryDate` and `ProbationEndDate`.
3. If confirmation happens in a later quarter and the retroactive rule is enabled, mark prior quarter eligibility as valid according to rule config.
4. Store the reason and source quarter for traceability.

### 13.2 Working Years Logic

1. Working years should be calculated from local employment start date.
2. The calculation reference date must be configurable.
3. Working years to leave days mapping must be configurable, not hard-coded.

### 13.3 Grant Logic

1. Grant annually entitled leave in quarterly portions.
2. If retroactive eligibility is confirmed later, create a retroactive grant record.
3. All grant records must be append-only ledger style whenever possible.
4. Do not overwrite historical records silently.

## 14. Idempotency and Consistency Requirements

This section is mandatory.

1. Every scheduled job must be idempotent.
2. Every grant action must be traceable to source eligibility result and rule version.
3. Every overtime credit must be traceable to approval and match result.
4. Approval rollback or cancellation must support compensatory rollback.
5. Recalculation must not corrupt historical ledgers.
6. Prefer append-only records plus status markers over destructive updates.

## 15. Performance Requirements

Because the goal is to reduce DingTalk API calls, performance should rely on local indexed queries.

Requirements:

1. Add indexes on `user_id`, `year`, `quarter`, `approval_id`, and date range fields.
2. Batch process users by chunks for recalculation jobs.
3. Avoid loading all attendance records of all users into memory at once.
4. Use time range filters in repositories.
5. Prefer incremental matching and incremental recalculation where possible.

## 16. Testing Requirements

The AI developer must add tests for the following cases.

### 16.1 Annual Leave Eligibility Tests

1. Employee joins before year start and confirms in Q1.
2. Employee confirms in Q3 and Q2 becomes retroactively eligible.
3. Employee has missing confirmation date.
4. Employee entry date changes and recalculation updates result correctly.

### 16.2 Annual Leave Grant Tests

1. Working years map to correct annual leave entitlement.
2. Quarterly split creates expected grant days.
3. Retroactive grant is created only once.
4. Re-running grant job does not duplicate records.

### 16.3 Overtime Matching Tests

1. Approved overtime with valid attendance credits correct minutes.
2. Approved overtime without attendance does not credit.
3. Attendance shorter than approved duration uses attendance result.
4. Re-running match job is idempotent.
5. Approval cancellation rolls back credited balance correctly.

## 17. Implementation Boundaries

The AI developer should not do the following in the first version:

1. Do not add real-time DingTalk queries in query APIs.
2. Do not make frontend pages depend on DingTalk response.
3. Do not hard-code leave entitlement values in handler functions.
4. Do not mix sync code with business grant logic in the same method.

## 18. Deliverables

The expected deliverables are:

1. New GORM models and migrations for the new local tables.
2. New repositories for the new tables.
3. New services for leave eligibility, leave grants, overtime matching, and compensatory balance.
4. New API handlers and routes.
5. Local-only scheduled jobs.
6. Unit tests for calculation and matching logic.
7. Minimal documentation update for how to run recalculation and grant jobs.

## 19. Suggested Implementation Plan

Phase 1:

1. Add data models and migrations.
2. Add repositories.
3. Add annual leave eligibility service and tests.

Phase 2:

1. Add leave rule config support.
2. Add annual leave grant service and tests.
3. Add leave-related APIs.

Phase 3:

1. Add overtime rule config support.
2. Add overtime matching service and compensatory ledger.
3. Add overtime-related APIs and tests.

Phase 4:

1. Add scheduled jobs.
2. Add rerun and rollback safeguards.
3. Verify no business API requires DingTalk calls.

## 20. Final Instruction to AI Developer

When implementing these requirements, always prefer local persisted state over external calls. Treat DingTalk as a sync source, not a runtime dependency. If a rule is ambiguous, implement it as a configurable policy and keep the first version conservative, traceable, and idempotent.
