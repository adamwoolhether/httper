package httper_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"time"

	"github.com/adamwoolhether/httper"
	"github.com/adamwoolhether/httper/client"
)

func ExampleNewClient() {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"msg":"hello"}`)
	}))
	defer ts.Close()

	c, err := httper.NewClient(client.WithTimeout(5 * time.Second))
	if err != nil {
		fmt.Println("build error:", err)
		return
	}

	u, _ := url.Parse(ts.URL)

	req, err := client.Request(context.Background(), u, http.MethodGet)
	if err != nil {
		fmt.Println("request error:", err)
		return
	}

	var resp struct{ Msg string }
	if err := c.Do(req, http.StatusOK, client.WithDestination(&resp)); err != nil {
		fmt.Println("do error:", err)
		return
	}

	fmt.Println(resp.Msg)
	// Output: hello
}
