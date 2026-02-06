package download

import (
	"encoding/hex"
	"fmt"
	"hash"
)

// checksumVerifier enables checksum validation of the downloaded file.
type checksumVerifier struct {
	hash     hash.Hash
	expected string
}

func (v *checksumVerifier) Write(p []byte) (int, error) {
	return v.hash.Write(p)
}

func (v *checksumVerifier) Verify() error {
	if v == nil {
		return nil
	}

	actual := hex.EncodeToString(v.hash.Sum(nil))
	if actual != v.expected {
		return &Error{
			Err:    ErrChecksumMismatch,
			Detail: fmt.Sprintf("expected %s, got %s", v.expected, actual),
		}
	}

	return nil
}
