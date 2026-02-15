package errs_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/adamwoolhether/httper/web/errs"
)

func TestNew(t *testing.T) {
	err := errs.New(http.StatusBadRequest, fmt.Errorf("bad input"))

	if err.Code != http.StatusBadRequest {
		t.Fatalf("Code = %d, want %d", err.Code, http.StatusBadRequest)
	}
	if err.Message != "bad input" {
		t.Fatalf("Message = %q, want %q", err.Message, "bad input")
	}
	if err.FuncName == "" {
		t.Fatal("FuncName should be populated by runtime.Caller")
	}
	if err.FileName == "" {
		t.Fatal("FileName should be populated by runtime.Caller")
	}
	if err.InnerErr {
		t.Fatal("InnerErr should be false for New")
	}
}

func TestNewInternal(t *testing.T) {
	err := errs.NewInternal(fmt.Errorf("db failure"))

	if err.Code != http.StatusInternalServerError {
		t.Fatalf("Code = %d, want %d", err.Code, http.StatusInternalServerError)
	}
	if err.Message != "db failure" {
		t.Fatalf("Message = %q, want %q", err.Message, "db failure")
	}
	if !err.InnerErr {
		t.Fatal("InnerErr should be true for NewInternal")
	}
	if err.FuncName == "" {
		t.Fatal("FuncName should be populated")
	}
	if !strings.Contains(err.FileName, "errors_test.go") {
		t.Fatalf("FileName = %q, want to contain errors_test.go", err.FileName)
	}
}

func TestError_Error(t *testing.T) {
	err := errs.New(http.StatusNotFound, fmt.Errorf("not found"))

	var e error = err
	if e.Error() != "not found" {
		t.Fatalf("Error() = %q, want %q", e.Error(), "not found")
	}
}

func TestError_IsInternal(t *testing.T) {
	regular := errs.New(http.StatusBadRequest, fmt.Errorf("bad"))
	internal := errs.NewInternal(fmt.Errorf("secret"))

	if regular.IsInternal() {
		t.Fatal("regular error should not be internal")
	}
	if !internal.IsInternal() {
		t.Fatal("internal error should be internal")
	}
}

func TestError_JSON(t *testing.T) {
	err := errs.New(http.StatusBadRequest, fmt.Errorf("invalid"))

	data, jsonErr := json.Marshal(err)
	if jsonErr != nil {
		t.Fatalf("json.Marshal: %v", jsonErr)
	}

	var m map[string]any
	if jsonErr := json.Unmarshal(data, &m); jsonErr != nil {
		t.Fatalf("json.Unmarshal: %v", jsonErr)
	}

	if m["code"].(float64) != float64(http.StatusBadRequest) {
		t.Fatalf("JSON code = %v, want %d", m["code"], http.StatusBadRequest)
	}
	if m["message"] != "invalid" {
		t.Fatalf("JSON message = %v, want %q", m["message"], "invalid")
	}
	if _, ok := m["FuncName"]; ok {
		t.Fatal("FuncName should be omitted from JSON")
	}
	if _, ok := m["FileName"]; ok {
		t.Fatal("FileName should be omitted from JSON")
	}
	if _, ok := m["InnerErr"]; ok {
		t.Fatal("InnerErr should be omitted from JSON")
	}
}

func TestError_AsType(t *testing.T) {
	inner := errs.New(http.StatusConflict, fmt.Errorf("conflict"))
	wrapped := fmt.Errorf("wrapping: %w", inner)

	var target *errs.Error
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find *errs.Error through wrapping")
	}
	if target.Code != http.StatusConflict {
		t.Fatalf("Code = %d, want %d", target.Code, http.StatusConflict)
	}
}

func TestNewFieldsError(t *testing.T) {
	err := errs.NewFieldsError("email", fmt.Errorf("required"))

	fe := errs.GetFieldErrors(err)
	if fe == nil {
		t.Fatal("expected FieldErrors, got nil")
	}
	if len(fe) != 1 {
		t.Fatalf("len = %d, want 1", len(fe))
	}
	if fe[0].Field != "email" {
		t.Fatalf("Field = %q, want %q", fe[0].Field, "email")
	}
	if fe[0].Err != "required" {
		t.Fatalf("Err = %q, want %q", fe[0].Err, "required")
	}
}

func TestFieldErrors_Error(t *testing.T) {
	fe := errs.NewFieldsError("name", fmt.Errorf("too short"))
	s := fe.Error()

	var arr []map[string]string
	if err := json.Unmarshal([]byte(s), &arr); err != nil {
		t.Fatalf("Error() should produce valid JSON: %v", err)
	}
	if len(arr) != 1 {
		t.Fatalf("len = %d, want 1", len(arr))
	}
	if arr[0]["field"] != "name" {
		t.Fatalf("field = %q, want %q", arr[0]["field"], "name")
	}
}

func TestFieldErrors_Fields(t *testing.T) {
	fe := errs.GetFieldErrors(errs.NewFieldsError("age", fmt.Errorf("must be positive")))

	m := fe.Fields()
	if v, ok := m["age"]; !ok || v != "must be positive" {
		t.Fatalf("Fields() = %v, want map with age=must be positive", m)
	}
}

func TestIsFieldErrors(t *testing.T) {
	fe := errs.NewFieldsError("x", fmt.Errorf("bad"))
	plain := fmt.Errorf("not field error")
	wrapped := fmt.Errorf("wrap: %w", fe)

	if !errs.IsFieldErrors(fe) {
		t.Fatal("IsFieldErrors should return true for FieldErrors")
	}
	if errs.IsFieldErrors(plain) {
		t.Fatal("IsFieldErrors should return false for plain error")
	}
	if !errs.IsFieldErrors(wrapped) {
		t.Fatal("IsFieldErrors should return true for wrapped FieldErrors")
	}
}

func TestGetFieldErrors(t *testing.T) {
	fe := errs.NewFieldsError("f", fmt.Errorf("e"))

	got := errs.GetFieldErrors(fe)
	if got == nil {
		t.Fatal("GetFieldErrors should extract FieldErrors")
	}

	if errs.GetFieldErrors(fmt.Errorf("plain")) != nil {
		t.Fatal("GetFieldErrors should return nil for non-FieldErrors")
	}
}
