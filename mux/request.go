package mux

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
)

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

func Param(r *http.Request, key string) (string, error) {
	val := r.PathValue(key)
	if val == "" {
		return "", fmt.Errorf("path param[%s] not found", key)
	}

	return val, nil
}

func QueryString(r *http.Request, key string) (string, error) {
	val := r.URL.Query().Get(key)
	if val == "" {
		return "", fmt.Errorf("query param[%s] is empty", key)
	}

	return val, nil
}

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

// DecodeAllowUnknownFields is the same as Decode, but wont' reject unknown fields.
func DecodeAllowUnknownFields[T any](r *http.Request, val *T) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	return nil
}
