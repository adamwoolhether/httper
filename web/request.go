package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Param extracts a path parameter by key and returns its string value.
func Param(r *http.Request, key string) (string, error) {
	val := r.PathValue(key)
	if val == "" {
		return "", fmt.Errorf("path param[%s] not found", key)
	}

	return val, nil
}

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

// QueryInt extracts a query parameter by key and parses it as an int.
func QueryInt(r *http.Request, key string) (int, error) {
	val := r.URL.Query().Get(key)
	if val == "" {
		return 0, fmt.Errorf("query param[%s] not found", key)
	}

	v, err := strconv.Atoi(val)
	if err != nil {
		return 0, fmt.Errorf("query param[%s] must be integer: %w", key, err)
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

// QueryStrings extracts every value of a query parameter by key. It reads
// repeated keys and splits each value on commas, so ?id=1,2&id=3 yields
// "1", "2", "3". An empty value, such as in ?id=1,,2, returns an error.
// A value cannot contain a comma, even when percent-encoded as %2C, because
// QueryStrings splits after decoding.
func QueryStrings(r *http.Request, key string) ([]string, error) {
	raw := r.URL.Query()[key]
	if len(raw) == 0 {
		return nil, fmt.Errorf("query param[%s] not found", key)
	}

	n := 0
	for _, v := range raw {
		n += strings.Count(v, ",") + 1
	}

	vals := make([]string, 0, n)
	for _, v := range raw {
		for s := range strings.SplitSeq(v, ",") {
			if s == "" {
				return nil, fmt.Errorf("query param[%s] has an empty value", key)
			}
			vals = append(vals, s)
		}
	}

	return vals, nil
}

// QueryBools is like QueryStrings, but parses each value as a bool.
func QueryBools(r *http.Request, key string) ([]bool, error) {
	return queryParse(r, key, "boolean", strconv.ParseBool)
}

// QueryInts is like QueryStrings, but parses each value as an int.
func QueryInts(r *http.Request, key string) ([]int, error) {
	return queryParse(r, key, "integer", strconv.Atoi)
}

// QueryInt64s is like QueryStrings, but parses each value as an int64.
func QueryInt64s(r *http.Request, key string) ([]int64, error) {
	return queryParse(r, key, "int64", func(s string) (int64, error) {
		return strconv.ParseInt(s, 10, 64)
	})
}

func queryParse[T any](r *http.Request, key, kind string, parse func(string) (T, error)) ([]T, error) {
	vals, err := QueryStrings(r, key)
	if err != nil {
		return nil, err
	}

	out := make([]T, len(vals))
	for i, v := range vals {
		out[i], err = parse(v)
		if err != nil {
			return nil, fmt.Errorf("query param[%s] must be %s: %w", key, kind, err)
		}
	}

	return out, nil
}

// Decode reads the body of an HTTP request looking for a JSON document. The
// body is decoded into the provided value.
// If the value is a struct or a slice of structs, Decode checks the validation
// tags of each struct. A field error in a slice element names the element
// index, such as "[1].name". A JSON null validates as an empty struct, and a
// null slice element fails as a required field. Decode does not validate
// other types.
// Decode does not limit the body size. Use DecodeLimit, or wrap r.Body
// with http.MaxBytesReader before calling Decode.
func Decode[T any](r *http.Request, val *T) error {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := validateDecoded(val); err != nil {
		return err
	}

	return nil
}

// DecodeAllowUnknownFields is the same as Decode, but won't reject unknown fields.
// Like Decode, it does not limit the body size.
func DecodeAllowUnknownFields[T any](r *http.Request, val *T) error {
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(val); err != nil {
		return fmt.Errorf("decode: %w", err)
	}

	if err := validateDecoded(val); err != nil {
		return err
	}

	return nil
}

// DecodeLimit is the same as Decode, but reads at most maxBytes of the body.
// A JSON value that does not fit in maxBytes returns an error that wraps
// *http.MaxBytesError.
func DecodeLimit[T any](w http.ResponseWriter, r *http.Request, val *T, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)

	return Decode(r, val)
}
