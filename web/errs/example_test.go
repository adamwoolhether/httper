package errs_test

import (
	"fmt"
	"net/http"

	"github.com/adamwoolhether/httper/web/errs"
)

// ————————————————————————————————————————————————————————————————————
// Error type examples
// ————————————————————————————————————————————————————————————————————

func ExampleNew() {
	err := errs.New(http.StatusNotFound, fmt.Errorf("user not found"))

	fmt.Println(err.Code)
	fmt.Println(err.Error())
	// Output:
	// 404
	// user not found
}

func ExampleNewInternal() {
	err := errs.NewInternal(fmt.Errorf("db connection lost"))

	fmt.Println(err.Code)
	fmt.Println(err.IsInternal())
	// Output:
	// 500
	// true
}

// ————————————————————————————————————————————————————————————————————
// Field error examples
// ————————————————————————————————————————————————————————————————————

func ExampleNewFieldsError() {
	err := errs.NewFieldsError("email", fmt.Errorf("invalid format"))

	fmt.Println(err)
	// Output: [{"field":"email","error":"invalid format"}]
}

func ExampleFieldErrors_Fields() {
	fe := errs.FieldErrors{
		{Field: "name", Err: "required"},
		{Field: "email", Err: "invalid"},
	}

	fields := fe.Fields()
	fmt.Println(fields["name"])
	fmt.Println(fields["email"])
	// Output:
	// required
	// invalid
}

func ExampleIsFieldErrors() {
	err := errs.NewFieldsError("age", fmt.Errorf("must be positive"))

	fmt.Println(errs.IsFieldErrors(err))
	fmt.Println(errs.IsFieldErrors(fmt.Errorf("other error")))
	// Output:
	// true
	// false
}

func ExampleGetFieldErrors() {
	err := errs.NewFieldsError("name", fmt.Errorf("required"))
	wrapped := fmt.Errorf("validation failed: %w", err)

	fe := errs.GetFieldErrors(wrapped)
	fmt.Println(fe.Fields()["name"])
	// Output: required
}
