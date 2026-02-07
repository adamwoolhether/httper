// Package httper exposes client builder.
package httper

import (
	"github.com/adamwoolhether/httper/client"
)

// NewClient instantiates a new *Client with the provided options.
// If not specified, the default htt.Client and htt.Transport are used.
func NewClient(opts ...client.Option) (*client.Client, error) {
	return client.Build(opts...)
}
