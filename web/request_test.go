package web_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
)

// ---- Param ----

func TestParam(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/users/alice", nil)
	r.SetPathValue("name", "alice")

	val, err := web.Param(r, "name")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "alice" {
		t.Fatalf("val = %q, want %q", val, "alice")
	}
}

func TestParam_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/users", nil)

	_, err := web.Param(r, "name")
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

// ---- ParamInt ----

func TestParamInt(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	r.SetPathValue("id", "42")

	val, err := web.ParamInt(r, "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 42 {
		t.Fatalf("val = %d, want 42", val)
	}
}

func TestParamInt_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items/abc", nil)
	r.SetPathValue("id", "abc")

	_, err := web.ParamInt(r, "id")
	if err == nil {
		t.Fatal("expected error for non-integer")
	}
}

func TestParamInt_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items", nil)

	_, err := web.ParamInt(r, "id")
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

// ---- ParamInt64 ----

func TestParamInt64(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items/9999999999", nil)
	r.SetPathValue("id", "9999999999")

	val, err := web.ParamInt64(r, "id")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 9999999999 {
		t.Fatalf("val = %d, want 9999999999", val)
	}
}

func TestParamInt64_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items/xyz", nil)
	r.SetPathValue("id", "xyz")

	_, err := web.ParamInt64(r, "id")
	if err == nil {
		t.Fatal("expected error for non-integer")
	}
}

func TestParamInt64_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items", nil)

	_, err := web.ParamInt64(r, "id")
	if err == nil {
		t.Fatal("expected error for missing param")
	}
}

// ---- QueryString ----

func TestQueryString(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/search?q=hello", nil)

	val, err := web.QueryString(r, "q")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != "hello" {
		t.Fatalf("val = %q, want %q", val, "hello")
	}
}

func TestQueryString_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/search", nil)

	_, err := web.QueryString(r, "q")
	if err == nil {
		t.Fatal("expected error for missing query param")
	}
}

// ---- QueryBool ----

func TestQueryBool(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?active=true", nil)

	val, err := web.QueryBool(r, "active")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != true {
		t.Fatal("val should be true")
	}
}

func TestQueryBool_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?active=maybe", nil)

	_, err := web.QueryBool(r, "active")
	if err == nil {
		t.Fatal("expected error for invalid bool")
	}
}

func TestQueryBool_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items", nil)

	_, err := web.QueryBool(r, "active")
	if err == nil {
		t.Fatal("expected error for missing query param")
	}
}

// ---- QueryInt ----

func TestQueryInt(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?page=3", nil)

	val, err := web.QueryInt(r, "page")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 3 {
		t.Fatalf("val = %d, want 3", val)
	}
}

func TestQueryInt_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?page=abc", nil)

	_, err := web.QueryInt(r, "page")
	if err == nil {
		t.Fatal("expected error for non-integer")
	}
}

func TestQueryInt_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items", nil)

	_, err := web.QueryInt(r, "page")
	if err == nil {
		t.Fatal("expected error for missing query param")
	}
}

// ---- QueryInt64 ----

func TestQueryInt64(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?offset=8888888888", nil)

	val, err := web.QueryInt64(r, "offset")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if val != 8888888888 {
		t.Fatalf("val = %d, want 8888888888", val)
	}
}

func TestQueryInt64_Invalid(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items?offset=nope", nil)

	_, err := web.QueryInt64(r, "offset")
	if err == nil {
		t.Fatal("expected error for non-integer")
	}
}

func TestQueryInt64_Missing(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/items", nil)

	_, err := web.QueryInt64(r, "offset")
	if err == nil {
		t.Fatal("expected error for missing query param")
	}
}

// ---- Decode ----

type testPayload struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

func TestDecode(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.Decode(r, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Alice" {
		t.Fatalf("Name = %q, want %q", p.Name, "Alice")
	}
	if p.Email != "alice@example.com" {
		t.Fatalf("Email = %q, want %q", p.Email, "alice@example.com")
	}
}

func TestDecode_InvalidJSON(t *testing.T) {
	body := `{bad json`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.Decode(r, &p); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDecode_UnknownFieldsRejected(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com","extra":"field"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.Decode(r, &p); err == nil {
		t.Fatal("expected error for unknown fields")
	}
}

func TestDecode_ValidationFailure(t *testing.T) {
	body := `{"name":"","email":"not-an-email"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	err := web.Decode(r, &p)
	if err == nil {
		t.Fatal("expected validation error")
	}
}

func TestDecodeAllowUnknownFields(t *testing.T) {
	body := `{"name":"Bob","email":"bob@example.com","extra":"field"}`
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.DecodeAllowUnknownFields(r, &p); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Bob" {
		t.Fatalf("Name = %q, want %q", p.Name, "Bob")
	}
}

func TestDecode_NonStructTargets(t *testing.T) {
	t.Run("map", func(t *testing.T) {
		got := mustDecode[map[string]any](t, `{"name":"Alice"}`)
		if got["name"] != "Alice" {
			t.Fatalf("name = %v, want %q", got["name"], "Alice")
		}
	})

	t.Run("string slice", func(t *testing.T) {
		got := mustDecode[[]string](t, `["a","b"]`)
		if len(got) != 2 || got[1] != "b" {
			t.Fatalf("got %v, want [a b]", got)
		}
	})

	t.Run("int", func(t *testing.T) {
		if got := mustDecode[int](t, `42`); got != 42 {
			t.Fatalf("got %d, want 42", got)
		}
	})

	t.Run("time", func(t *testing.T) {
		want := time.Date(2026, 10, 5, 0, 0, 0, 0, time.UTC)
		if got := mustDecode[time.Time](t, `"2026-10-05T00:00:00Z"`); !got.Equal(want) {
			t.Fatalf("got %v, want %v", got, want)
		}
	})

	t.Run("allow unknown fields map", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"Alice"}`))

		var got map[string]any
		if err := web.DecodeAllowUnknownFields(r, &got); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["name"] != "Alice" {
			t.Fatalf("name = %v, want %q", got["name"], "Alice")
		}
	})
}

func TestDecode_PointerToStruct(t *testing.T) {
	got := mustDecode[*testPayload](t, `{"name":"Alice","email":"alice@example.com"}`)
	if got == nil || got.Name != "Alice" {
		t.Fatalf("got %+v, want Name %q", got, "Alice")
	}

	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"name":"","email":"alice@example.com"}`))

	var p *testPayload
	fields := errs.GetFieldErrors(web.Decode(r, &p)).Fields()
	if fields["name"] != "This field is required" {
		t.Fatalf("field errors = %v, want name required", fields)
	}
}

func TestDecode_StructSlice(t *testing.T) {
	valid := `[{"name":"Alice","email":"alice@example.com"},{"name":"Bob","email":"bob@example.com"}]`
	invalid := `[{"name":"Alice","email":"alice@example.com"},{"name":"","email":"not-an-email"}]`

	t.Run("valid", func(t *testing.T) {
		if got := mustDecode[[]testPayload](t, valid); len(got) != 2 || got[1].Name != "Bob" {
			t.Fatalf("got %+v, want two payloads", got)
		}
	})

	t.Run("invalid element", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(invalid))

		var got []testPayload
		fields := errs.GetFieldErrors(web.Decode(r, &got)).Fields()
		if fields["[1].name"] != "This field is required" {
			t.Fatalf("field errors = %v, want [1].name required", fields)
		}
		if _, ok := fields["[1].email"]; !ok {
			t.Fatalf("field errors = %v, want [1].email", fields)
		}
		if len(fields) != 2 {
			t.Fatalf("field errors = %v, want only element 1 errors", fields)
		}
	})

	t.Run("invalid pointer element", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(invalid))

		var got []*testPayload
		fields := errs.GetFieldErrors(web.Decode(r, &got)).Fields()
		if fields["[1].name"] != "This field is required" {
			t.Fatalf("field errors = %v, want [1].name required", fields)
		}
	})
}

func mustDecode[T any](t *testing.T, body string) T {
	t.Helper()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var v T
	if err := web.Decode(r, &v); err != nil {
		t.Fatalf("Decode into %T: %v", v, err)
	}

	return v
}

func TestDecodeLimit(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.DecodeLimit(w, r, &p, int64(len(body))); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Name != "Alice" {
		t.Fatalf("Name = %q, want %q", p.Name, "Alice")
	}
}

func TestDecodeLimit_BodyTooLarge(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com"}`
	limit := int64(len(body)) - 1
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	err := web.DecodeLimit(w, r, &p, limit)

	maxErr, ok := errors.AsType[*http.MaxBytesError](err)
	if !ok {
		t.Fatalf("err = %v, want *http.MaxBytesError", err)
	}
	if maxErr.Limit != limit {
		t.Fatalf("Limit = %d, want %d", maxErr.Limit, limit)
	}
}

func TestDecodeLimit_UnknownFieldsRejected(t *testing.T) {
	body := `{"name":"Alice","email":"alice@example.com","extra":"field"}`
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))

	var p testPayload
	if err := web.DecodeLimit(w, r, &p, 1<<20); err == nil {
		t.Fatal("expected error for unknown fields")
	}
}
