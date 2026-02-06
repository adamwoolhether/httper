package throttle_test

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/adamwoolhether/httper/client/throttle"
)

func ExampleNewRoundTripper() {
	rt, err := throttle.NewRoundTripper(
		10, // requests per second
		5,  // burst capacity
		func() *slog.Logger { return slog.Default() },
		http.DefaultTransport,
	)
	if err != nil {
		fmt.Println("error:", err)
		return
	}

	_ = &http.Client{Transport: rt}

	fmt.Println("throttled transport created")
	// Output: throttled transport created
}
