// Package errs enables error handling and definition at the http/app level.
package errs

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"runtime"
)

// Error represents an error in the system.
type Error struct {
	Code     int    `json:"code"`
	Message  string `json:"message"`
	FuncName string `json:"-"`
	FileName string `json:"-"`
	InnerErr bool   `json:"-"`
}

// New constructs an error based on an app error.
func New(code int, err error) *Error {
	pc, filename, line, _ := runtime.Caller(1)

	return &Error{
		Code:     code,
		Message:  err.Error(),
		FuncName: runtime.FuncForPC(pc).Name(),
		FileName: fmt.Sprintf("%s:%d", filename, line),
	}
}

// NewInternal creates an error that is not intended
// to be seen by users.
func NewInternal(err error) *Error {
	pc, filename, line, _ := runtime.Caller(1)

	return &Error{
		Code:     http.StatusInternalServerError,
		Message:  err.Error(),
		FuncName: runtime.FuncForPC(pc).Name(),
		FileName: fmt.Sprintf("%s:%d", filename, line),
		InnerErr: true,
	}
}

// Error implements the error interface.
func (e *Error) Error() string {
	return e.Message
}

// IsInternal returns true if the error is internal.
func (e *Error) IsInternal() bool {
	return e.InnerErr
}

// /////////////////////////////////////////////////////////////////////////////////////////////

// FieldError is used to indicate an error with a specific request field.
type FieldError struct {
	Field string `json:"field"`
	Err   string `json:"error"`
}

// FieldErrors represents a collection of field errors.
type FieldErrors []FieldError

// NewFieldsError creates a fields error.
func NewFieldsError(field string, err error) error {
	return FieldErrors{
		{
			Field: field,
			Err:   err.Error(),
		},
	}
}

// Error implements the error interface.
func (fe FieldErrors) Error() string {
	d, err := json.Marshal(fe)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

// Fields returns the fields that failed validation
func (fe FieldErrors) Fields() map[string]string {
	m := make(map[string]string)
	for _, fld := range fe {
		m[fld.Field] = fld.Err
	}
	return m
}

// IsFieldErrors checks if an error of type FieldErrors exists.
func IsFieldErrors(err error) bool {
	var fe FieldErrors
	return errors.As(err, &fe)
}

// GetFieldErrors returns a copy of the FieldErrors pointer.
func GetFieldErrors(err error) FieldErrors {
	var fe FieldErrors
	if !errors.As(err, &fe) {
		return nil
	}
	return fe
}
