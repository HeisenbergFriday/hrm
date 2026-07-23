package api

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestParseWeekSchedulePushUserIDsJSONArray(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("user_ids", `["u1"," u2 ","" ,"u1"]`)
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/week-schedule/push/personal", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse form: %v", err)
	}

	ids, err := parseWeekSchedulePushUserIDs(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != "u1" || ids[1] != "u2" || ids[2] != "u1" {
		// parser does not de-dupe; service layer does. Accept raw order with empties stripped.
		t.Fatalf("ids=%v", ids)
	}
}

func TestParseWeekSchedulePushUserIDsCommaSeparated(t *testing.T) {
	gin.SetMode(gin.TestMode)
	body := &bytes.Buffer{}
	w := multipart.NewWriter(body)
	_ = w.WriteField("user_ids", "a, b , ,c")
	_ = w.Close()

	req := httptest.NewRequest(http.MethodPost, "/", body)
	req.Header.Set("Content-Type", w.FormDataContentType())
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = req
	if err := c.Request.ParseMultipartForm(1 << 20); err != nil {
		t.Fatalf("parse form: %v", err)
	}
	ids, err := parseWeekSchedulePushUserIDs(c)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(ids) != 3 || ids[0] != "a" || ids[1] != "b" || ids[2] != "c" {
		t.Fatalf("ids=%v", ids)
	}
}
