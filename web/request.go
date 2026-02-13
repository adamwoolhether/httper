package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

// ParamInt extracts a path parameter by key and parses it as an int.
func ParamInt(r *http.Request, key string) (int, error) {
	val := r.PathValue(key)
	if val == "" {
		return 0, fmt.Errorf("path param[%s] not found", key)
	}

	v, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("path param[%s] must be integer: %w", key, err)
	}

	return v, nil
}

// ParamInt64 extracts a path parameter by key and parses it as an int64.
func ParamInt64(r *http.Request, key string) (int64, error) {
	val := r.PathValue(key)
	if val == "" {
		return 0, fmt.Errorf("path param[%s] not found", key)
	}

	v, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("path param[%s] must be integer: %w", key, err)
	}

	return v, nil
}

// Param extracts a path parameter by key and returns its string value.
func Param(r *http.Request, key string) (string, error) {
	val := r.PathValue(key)
	if val == "" {
		return "", fmt.Errorf("path param[%s] not found", key)
	}

	return val, nil
}

// QueryString extracts a query parameter by key and returns its string value.
func QueryString(r *http.Request, key string) (string, error) {
	val := r.URL.Query().Get(key)
	if val == "" {
		return "", fmt.Errorf("query param[%s] is empty", key)
	}

	return val, nil
}

// QueryBool extracts a query parameter by key and parses it as a bool.
func QueryBool(r *http.Request, key string) (bool, error) {
	val := r.URL.Query().Get(key)
	if val == "" {
		return false, fmt.Errorf("query param[%s] not found", key)
	}

	v, err := strconv.ParseBool(val)
	if err != nil {
		return false, fmt.Errorf("query param[%s] must be boolean: %w", key, err)
	}

	return v, nil
}

// QueryInt64 extracts a query parameter by key and parses it as an int64.
func QueryInt64(r *http.Request, key string) (int64, error) {
	val := r.URL.Query().Get(key)
	if val == "" {
		return 0, fmt.Errorf("query param[%s] not found", key)
	}

	v, err := strconv.ParseInt(val, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("query param[%s] must be int64: %w", key, err)
	}

	return v, nil
}

// Decode reads the body of an HTTP request looking for a JSON document. The
// body is decoded into the provided value.
// If the provided value is a struct then it is checked for validation tags.
// If the value implements a validate function, it is executed.
func Decode[T any](r *http.Request, val *T) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := Validate(val); err != nil {
		return err
	}

	return nil
}

// DecodeAllowUnknownFields is the same as Decode, but won't reject unknown fields.
func DecodeAllowUnknownFields[T any](r *http.Request, val *T) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := Validate(val); err != nil {
		return err
	}

	return nil
}
