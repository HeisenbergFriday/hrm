package service

import (
	"archive/zip"
	"bytes"
	"database/sql/driver"
	"encoding/json"
	"testing"
)

func TestParseXiaotieScoreDraftRescalesWeightsAndWarns(t *testing.T) {
	sheet := performanceImportWorkbookSheet{Name: "1.1目标责任书&评估表", Rows: []xlsxImportRow{
		{Number: 5, Values: []string{"PARTB: 个人绩效（员工绩效）"}},
		{Number: 8, Values: []string{"量化指标（2-5项，权重70%）", "", "", "0.3", "", "", "", "量化规则"}},
		{Number: 9, Values: []string{"", "", "", "0.2"}},
		{Number: 10, Values: []string{"", "", "", "0.1"}},
		{Number: 11, Values: []string{"关键行动（3-5项，权重30%）", "", "", "0.2", "行动说明"}},
		{Number: 12, Values: []string{"", "", "", "0.15", "行动说明"}},
		{Number: 13, Values: []string{"", "", "", "0.05", "行动说明"}},
		{Number: 14, Values: []string{"合计：", "", "", "1"}},
	}}

	draft, issues := parseXiaotieScoreDraft(sheet)
	if len(draft.Sections) != 2 {
		t.Fatalf("sections = %d, want 2", len(draft.Sections))
	}
	if draft.Sections[0].Weight != 70 || draft.Sections[1].Weight != 30 {
		t.Fatalf("section weights = %.2f/%.2f, want 70/30", draft.Sections[0].Weight, draft.Sections[1].Weight)
	}
	goalTotal := 0.0
	for _, goal := range draft.Goals {
		goalTotal += goal.Weight
	}
	if performanceImportAbs(goalTotal-100) > 0.01 {
		t.Fatalf("goal total = %.4f, want 100", goalTotal)
	}
	for index, goal := range draft.Goals {
		if goal.SortOrder != index+1 {
			t.Fatalf("goal %d sort order = %d, want %d", index, goal.SortOrder, index+1)
		}
	}
	keyAction := draft.Goals[len(draft.Goals)-1]
	if keyAction.ScoringRule != "\u884c\u52a8\u8bf4\u660e" || keyAction.RedLineValue != "" {
		t.Fatalf("key action mapping = scoring rule %q, red line %q", keyAction.ScoringRule, keyAction.RedLineValue)
	}
	codes := map[string]bool{}
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	if !codes["section_weight_conflict"] || !codes["weight_below_template_minimum"] {
		t.Fatalf("issues = %#v, want section conflict and minimum warning", issues)
	}
}

func TestParseMutengPerformanceImportAllowsBonusWeight(t *testing.T) {
	svc := &PerformanceService{}
	sheet := performanceImportWorkbookSheet{Name: "绩效考核模板", Rows: []xlsxImportRow{
		{Number: 20, Values: []string{"下季度目标计划（自评时同步填写）"}},
		{Number: 22, Values: []string{"序号", "目标/关键职责事项", "权重", "说明"}},
		{Number: 23, Values: []string{"1", "价值观及工作纪律", "0.15", "固定项"}},
		{Number: 24, Values: []string{"2", "端到端转型", "0.25", "说明"}},
		{Number: 25, Values: []string{"3", "任务积点", "0.5", "说明"}},
		{Number: 26, Values: []string{"4", "文档记录", "0.1", "说明"}},
		{Number: 27, Values: []string{"6", "加分项（非必填）", "0.1", "特殊贡献"}},
		{Number: 28, Values: []string{"", "合计", "1.1"}},
	}}
	preview, err := svc.parseMutengPerformanceImport("模板.xlsx", []performanceImportWorkbookSheet{sheet})
	if err != nil {
		t.Fatal(err)
	}
	draft := preview.Drafts[0]
	if draft.SourceWeightTotal != 110 {
		t.Fatalf("source total = %.2f, want 110", draft.SourceWeightTotal)
	}
	if !draft.EnableBonusScore || len(draft.Sections) != 2 {
		t.Fatalf("bonus=%v sections=%d", draft.EnableBonusScore, len(draft.Sections))
	}
	if draft.Sections[0].Weight != 100 || draft.Sections[1].Weight != 0 {
		t.Fatalf("section weights = %.2f/%.2f, want 100/0", draft.Sections[0].Weight, draft.Sections[1].Weight)
	}
}

func TestReadPerformanceImportWorkbookSheetsIgnoresMissingSheetFile(t *testing.T) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	writeZip := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	writeZip("xl/workbook.xml", `<?xml version="1.0"?><workbook xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="有效" sheetId="1" r:id="rId1"/><sheet name="坏引用" sheetId="2" r:id="rId2"/></sheets></workbook>`)
	writeZip("xl/_rels/workbook.xml.rels", `<?xml version="1.0"?><Relationships><Relationship Id="rId1" Target="worksheets/sheet1.xml"/><Relationship Id="rId2" Target="worksheets/missing.xml"/></Relationships>`)
	writeZip("xl/worksheets/sheet1.xml", `<?xml version="1.0"?><worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>ok</t></is></c></row></sheetData></worksheet>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	if err != nil {
		t.Fatal(err)
	}
	sheets, issues, err := readPerformanceImportWorkbookSheets(zr, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 || sheets[0].Name != "有效" {
		t.Fatalf("sheets = %#v", sheets)
	}
	if len(issues) != 1 || issues[0].Code != "missing_sheet_file" {
		t.Fatalf("issues = %#v", issues)
	}
}

func TestCommitPerformanceActivityImportReturnsCommittedResultIdempotently(t *testing.T) {
	result := PerformanceActivityImportCommitResult{BatchID: "batch-committed", Created: []PerformanceImportCreatedResult{{DraftKey: "draft-1", TemplateID: 10, ActivityID: 20}}}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	svc := newImportStubPerformanceService(t, importStubTableResponse(
		"performance_import_batches",
		[]string{"id", "org_id", "batch_key", "status", "preview_json", "result_json", "file_name", "file_sha256", "source_type", "created_by"},
		[][]driver.Value{{1, "org-test", "batch-committed", "committed", "{}", string(resultJSON), "test.xlsx", "hash", "xiaotie", "tester"}},
	))
	svc.orgID = "org-test"

	first, err := svc.CommitPerformanceActivityImport("batch-committed", PerformanceActivityImportCommitRequest{}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	second, err := svc.CommitPerformanceActivityImport("batch-committed", PerformanceActivityImportCommitRequest{}, "tester")
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Created) != 1 || len(second.Created) != 1 || first.Created[0].ActivityID != second.Created[0].ActivityID {
		t.Fatalf("first=%#v second=%#v", first, second)
	}
}

func TestMergePerformanceImportCommitDraftsRejectsUnknownDraft(t *testing.T) {
	source := []PerformanceImportActivityDraft{{DraftKey: "known", Selected: true}}
	_, err := mergePerformanceImportCommitDrafts(source, []PerformanceImportCommitDraft{{DraftKey: "unknown", Selected: true}})
	if err == nil {
		t.Fatal("expected unknown draft error")
	}
}

func TestValidatePerformanceImportDraftRequiresConfirmedDates(t *testing.T) {
	draft := PerformanceImportActivityDraft{
		TemplateName: "模板", ActivityName: "活动", CycleType: "quarterly",
		Sections: []PerformanceImportSectionDraft{{Name: "指标", Weight: 100}},
	}
	if err := validatePerformanceImportDraft(draft); err == nil {
		t.Fatal("expected date validation error")
	}
}
