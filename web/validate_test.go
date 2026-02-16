package web_test

import (
	"testing"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
)

type validStruct struct {
	Name  string `json:"name" validate:"required"`
	Email string `json:"email" validate:"required,email"`
}

func TestValidate_Valid(t *testing.T) {
	v := validStruct{Name: "Alice", Email: "alice@example.com"}
	if err := web.Validate(&v); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}
}

func TestValidate_MissingRequired(t *testing.T) {
	v := validStruct{Email: "alice@example.com"}
	err := web.Validate(&v)
	if err == nil {
		t.Fatal("expected error for missing required field")
	}

	fe := errs.GetFieldErrors(err)
	if fe == nil {
		t.Fatal("expected FieldErrors")
	}

	fields := fe.Fields()
	if _, ok := fields["name"]; !ok {
		t.Fatalf("expected 'name' field error, got %v", fields)
	}
	if fields["name"] != "This field is required" {
		t.Fatalf("name error = %q, want %q", fields["name"], "This field is required")
	}
}

func TestValidate_InvalidField(t *testing.T) {
	v := validStruct{Name: "Alice", Email: "not-an-email"}
	err := web.Validate(&v)
	if err == nil {
		t.Fatal("expected error for invalid email")
	}

	fe := errs.GetFieldErrors(err)
	if fe == nil {
		t.Fatal("expected FieldErrors")
	}

	fields := fe.Fields()
	if _, ok := fields["email"]; !ok {
		t.Fatalf("expected 'email' field error, got %v", fields)
	}
}

func TestValidate_NonStruct(t *testing.T) {
	s := "just a string"
	// Passing a non-struct should return nil (the validator.Struct call
	// returns an InvalidValidationError which is not ValidationErrors).
	err := web.Validate(&s)
	if err == nil {
		// Non-struct might return an error from the validator itself.
		// Either way, we just ensure it doesn't panic.
		return
	}
}
