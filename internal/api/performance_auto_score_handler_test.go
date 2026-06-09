package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"peopleops/internal/service"

	"github.com/gin-gonic/gin"
)

func TestAutoScoreGoalRecordsHandlerReturnsScoreResponse(t *testing.T) {
	items := []service.AutoItemInput{
		{
			RecordID:     1,
			SectionType:  "quantitative",
			Weight:       0.8,
			ScoringRule:  "threshold",
			TargetValue:  "100",
			ActualResult: "100",
		},
		{
			RecordID:     2,
			SectionType:  "bonus_penalty",
			Weight:       99,
			ScoringRule:  "threshold",
			TargetValue:  "10",
			ActualResult: "10",
		},
	}

	recorder := performAutoScoreRequest(t, map[string]interface{}{"items": items})

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	var response struct {
		Code    int                       `json:"code"`
		Message string                    `json:"message"`
		Data    service.AutoScoreResponse `json:"data"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if response.Code != http.StatusOK || response.Message != "success" {
		t.Fatalf("response status = (%d, %q), want (200, success)", response.Code, response.Message)
	}
	if len(response.Data.Items) != 2 {
		t.Fatalf("items length = %d, want 2", len(response.Data.Items))
	}
	if response.Data.Items[0].RecordID != 1 || response.Data.Items[0].Score != 100 || !response.Data.Items[0].AutoScored {
		t.Fatalf("first item = %#v", response.Data.Items[0])
	}
	if response.Data.Items[1].RecordID != 2 || response.Data.Items[1].Score != 100 || !response.Data.Items[1].AutoScored {
		t.Fatalf("bonus item = %#v", response.Data.Items[1])
	}
	if response.Data.TotalScore != 80 {
		t.Fatalf("total_score = %v, want 80", response.Data.TotalScore)
	}
}

func TestAutoScoreGoalRecordsHandlerRejectsInvalidRequests(t *testing.T) {
	tests := []struct {
		name string
		body string
	}{
		{
			name: "malformed json",
			body: `{"items":`,
		},
		{
			name: "missing required items",
			body: `{}`,
		},
		{
			name: "too many items",
			body: marshalAutoScorePayload(t, makeAutoScoreItems(101)),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := performRawAutoScoreRequest(tt.body)

			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusBadRequest, recorder.Body.String())
			}
		})
	}
}

func performAutoScoreRequest(t *testing.T, payload map[string]interface{}) *httptest.ResponseRecorder {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal auto score payload: %v", err)
	}
	return performRawAutoScoreRequest(string(body))
}

func performRawAutoScoreRequest(body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)

	recorder := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(recorder)
	c.Request = httptest.NewRequest(http.MethodPost, "/performance/auto-score", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")

	AutoScoreGoalRecordsHandler(c)
	return recorder
}

func marshalAutoScorePayload(t *testing.T, items []service.AutoItemInput) string {
	t.Helper()

	body, err := json.Marshal(map[string]interface{}{"items": items})
	if err != nil {
		t.Fatalf("marshal auto score payload: %v", err)
	}
	return string(bytes.TrimSpace(body))
}

func makeAutoScoreItems(count int) []service.AutoItemInput {
	items := make([]service.AutoItemInput, count)
	for i := range items {
		items[i] = service.AutoItemInput{
			RecordID:     uint(i + 1),
			SectionType:  "quantitative",
			Weight:       1,
			ScoringRule:  "threshold",
			TargetValue:  "100",
			ActualResult: "100",
		}
	}
	return items
}
