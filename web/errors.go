package web

import (
	"encoding/json"
	"errors"
)

// Error represents a recognized http Error.
type Error struct {
	Code    int   `json:"code"`
	Message error `json:"message"`
}

func (err Error) Error() string {
	return err.Message.Error()
}

// MarshalJSON implements json.Marshaler, encoding Message as its string
// representation so the error text appears in JSON output.
func (err Error) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	}{
		Code:    err.Code,
		Message: err.Message.Error(),
	})
}

func (err Error) Unwrap() error {
	return err.Message
}

// NewError creates a new web Error to be propagated up the handler
// stack and caught by middleware.
func NewError(code int, err error) Error {
	return Error{
		Code:    code,
		Message: err,
	}
}

// GetError attempts to retrieve the recognized error, or returns
// false if the error is unexpected.
func GetError(err error) (Error, bool) {
	var er Error
	if !errors.As(err, &er) {
		return Error{}, false
	}

	return er, true
}
