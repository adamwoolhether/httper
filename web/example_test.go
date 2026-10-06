package web_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/adamwoolhether/httper/web"
	"github.com/adamwoolhether/httper/web/errs"
)

// ————————————————————————————————————————————————————————————————————
// Request helper examples
// ————————————————————————————————————————————————————————————————————

func ExampleParam() {
	r := httptest.NewRequest(http.MethodGet, "/users/alice", nil)
	r.SetPathValue("name", "alice")

	name, err := web.Param(r, "name")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(name)
	// Output: alice
}

func ExampleParamInt() {
	r := httptest.NewRequest(http.MethodGet, "/items/42", nil)
	r.SetPathValue("id", "42")

	id, err := web.ParamInt(r, "id")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(id)
	// Output: 42
}

func ExampleParamInt64() {
	r := httptest.NewRequest(http.MethodGet, "/orders/9000000000", nil)
	r.SetPathValue("id", "9000000000")

	id, err := web.ParamInt64(r, "id")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(id)
	// Output: 9000000000
}

func ExampleQueryString() {
	r := httptest.NewRequest(http.MethodGet, "/search?q=golang", nil)

	q, err := web.QueryString(r, "q")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(q)
	// Output: golang
}

func ExampleQueryBool() {
	r := httptest.NewRequest(http.MethodGet, "/items?active=true", nil)

	active, err := web.QueryBool(r, "active")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(active)
	// Output: true
}

func ExampleQueryInt() {
	r := httptest.NewRequest(http.MethodGet, "/items?page=3", nil)

	page, err := web.QueryInt(r, "page")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(page)
	// Output: 3
}

func ExampleQueryInt64() {
	r := httptest.NewRequest(http.MethodGet, "/items?cursor=8000000000", nil)

	cursor, err := web.QueryInt64(r, "cursor")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(cursor)
	// Output: 8000000000
}

func ExampleQueryStrings() {
	r := httptest.NewRequest(http.MethodGet, "/items?tag=go,http&tag=web", nil)

	tags, err := web.QueryStrings(r, "tag")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(tags)
	// Output: [go http web]
}

func ExampleQueryBools() {
	r := httptest.NewRequest(http.MethodGet, "/items?flag=true,false", nil)

	flags, err := web.QueryBools(r, "flag")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(flags)
	// Output: [true false]
}

func ExampleQueryInts() {
	r := httptest.NewRequest(http.MethodGet, "/items?id=1,2&id=3", nil)

	ids, err := web.QueryInts(r, "id")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(ids)
	// Output: [1 2 3]
}

func ExampleQueryInt64s() {
	r := httptest.NewRequest(http.MethodGet, "/items?id=8000000000&id=8000000001", nil)

	ids, err := web.QueryInt64s(r, "id")
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(ids)
	// Output: [8000000000 8000000001]
}

// ————————————————————————————————————————————————————————————————————
// Decode examples
// ————————————————————————————————————————————————————————————————————

func ExampleDecode() {
	type Input struct {
		Name string `json:"name" validate:"required"`
	}

	body := strings.NewReader(`{"name":"alice"}`)
	r := httptest.NewRequest(http.MethodPost, "/", body)

	var input Input
	if err := web.Decode(r, &input); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(input.Name)
	// Output: alice
}

func ExampleDecodeAllowUnknownFields() {
	type Input struct {
		Name string `json:"name" validate:"required"`
	}

	body := strings.NewReader(`{"name":"alice","extra":"ignored"}`)
	r := httptest.NewRequest(http.MethodPost, "/", body)

	var input Input
	if err := web.DecodeAllowUnknownFields(r, &input); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(input.Name)
	// Output: alice
}

func ExampleDecodeLimit() {
	type Input struct {
		Name string `json:"name" validate:"required"`
	}

	body := strings.NewReader(`{"name":"alice"}`)
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/", body)

	var input Input
	if err := web.DecodeLimit(w, r, &input, 1<<20); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(input.Name)
	// Output: alice
}

// ————————————————————————————————————————————————————————————————————
// Response helper examples
// ————————————————————————————————————————————————————————————————————

func ExampleRespondJSON() {
	w := httptest.NewRecorder()

	data := map[string]string{"status": "ok"}
	if err := web.RespondJSON(context.Background(), w, http.StatusOK, data); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 200
	// {"status":"ok"}
}

func ExampleRespondError() {
	w := httptest.NewRecorder()

	appErr := errs.New(http.StatusNotFound, fmt.Errorf("user not found"))
	if err := web.RespondError(context.Background(), w, appErr); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(w.Code)
	fmt.Println(w.Body.String())
	// Output:
	// 404
	// {"code":404,"message":"user not found"}
}

func ExampleRedirect() {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/old", nil)

	if err := web.Redirect(w, r, "/new", http.StatusMovedPermanently); err != nil {
		fmt.Println("error:", err)
		return
	}

	fmt.Println(w.Code)
	fmt.Println(w.Header().Get("Location"))
	// Output:
	// 301
	// /new
}

// ————————————————————————————————————————————————————————————————————
// Validation examples
// ————————————————————————————————————————————————————————————————————

func ExampleValidate() {
	type Input struct {
		Name  string `json:"name" validate:"required"`
		Email string `json:"email" validate:"required,email"`
	}

	input := Input{
		Name:  "alice",
		Email: "alice@example.com",
	}

	err := web.Validate(&input)
	fmt.Println(err)
	// Output: <nil>
}

func ExampleValidate_errors() {
	type Input struct {
		Name string `json:"name" validate:"required"`
	}

	err := web.Validate(&Input{})

	fe := errs.GetFieldErrors(err)
	for _, f := range fe {
		fmt.Printf("%s: %s\n", f.Field, f.Err)
	}
	// Output: name: This field is required
}
