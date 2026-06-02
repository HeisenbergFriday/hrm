package service

import (
	"archive/zip"
	"bytes"
	"testing"
)

func TestParsePerformanceParticipantImportXLSXTemplate(t *testing.T) {
	data := buildTestParticipantImportXLSX(t)

	result, err := ParsePerformanceParticipantImportXLSX(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("ParsePerformanceParticipantImportXLSX() error = %v", err)
	}

	if result.ActivityName != "2026年Q2绩效考核" {
		t.Fatalf("ActivityName = %q", result.ActivityName)
	}
	if result.ParsedCount != 2 {
		t.Fatalf("ParsedCount = %d", result.ParsedCount)
	}
	if result.DuplicateCount != 1 {
		t.Fatalf("DuplicateCount = %d", result.DuplicateCount)
	}
	want := []string{"00123", "456"}
	if len(result.EmployeeIDs) != len(want) {
		t.Fatalf("EmployeeIDs length = %d, want %d: %#v", len(result.EmployeeIDs), len(want), result.EmployeeIDs)
	}
	for i := range want {
		if result.EmployeeIDs[i] != want[i] {
			t.Fatalf("EmployeeIDs[%d] = %q, want %q", i, result.EmployeeIDs[i], want[i])
		}
	}
}

func buildTestParticipantImportXLSX(t *testing.T) []byte {
	t.Helper()

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"xl/workbook.xml": `<?xml version="1.0" encoding="UTF-8"?>
<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <sheets>
    <sheet name="Sheet1" sheetId="1" r:id="rId1"/>
  </sheets>
</workbook>`,
		"xl/_rels/workbook.xml.rels": `<?xml version="1.0" encoding="UTF-8"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>
</Relationships>`,
		"xl/sharedStrings.xml": `<?xml version="1.0" encoding="UTF-8"?>
<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" count="12" uniqueCount="12">
  <si><t>绩效活动</t></si>
  <si><t>工号</t></si>
  <si><t>姓名</t></si>
  <si><t>一级部门</t></si>
  <si><t>二级部门</t></si>
  <si><t>三级部门</t></si>
  <si><t>2026年Q2绩效考核</t></si>
  <si><t>00123</t></si>
  <si><t>张三</t></si>
  <si><t>总部</t></si>
  <si><t>产品部</t></si>
  <si><t>平台组</t></si>
</sst>`,
		"xl/worksheets/sheet1.xml": `<?xml version="1.0" encoding="UTF-8"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <sheetData>
    <row r="2">
      <c r="A2" t="s"><v>0</v></c>
      <c r="B2" t="s"><v>1</v></c>
      <c r="C2" t="s"><v>2</v></c>
      <c r="D2" t="s"><v>3</v></c>
      <c r="E2" t="s"><v>4</v></c>
      <c r="F2" t="s"><v>5</v></c>
    </row>
    <row r="3">
      <c r="A3" t="s"><v>6</v></c>
      <c r="B3" t="s"><v>7</v></c>
      <c r="C3" t="s"><v>8</v></c>
      <c r="D3" t="s"><v>9</v></c>
      <c r="E3" t="s"><v>10</v></c>
      <c r="F3" t="s"><v>11</v></c>
    </row>
    <row r="4">
      <c r="A4" t="s"><v>6</v></c>
      <c r="B4" t="s"><v>7</v></c>
    </row>
    <row r="5">
      <c r="A5" t="s"><v>6</v></c>
      <c r="B5"><v>456.0</v></c>
    </row>
  </sheetData>
</worksheet>`,
	}
	for name, content := range files {
		writer, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create zip entry %s: %v", name, err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatalf("write zip entry %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}
	return buf.Bytes()
}
