package signalr

import (
	"fmt"
)

type (
	// SendFailureError provides added error information when a send message call fails
	SendFailureError struct {
		StatusCode int
		Body       string
	}
)

func (sfe SendFailureError) Error() string {
	return fmt.Sprintf("failed to send message with status code %d and body: %q\n", sfe.StatusCode, sfe.Body)
}
