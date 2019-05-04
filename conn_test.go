package signalr

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseConnectionString(t *testing.T) {
	connStr := "Endpoint=https://signalr-go-tests-tmycpkux.service.signalr.net;AccessKey=foo+bar+baz=;Version=1.0;"
	parsed, err := ParseConnectionString(connStr)
	assert.NoError(t, err)
	assert.Equal(t, "https://signalr-go-tests-tmycpkux.service.signalr.net", parsed.Endpoint)
	assert.Equal(t, "foo+bar+baz=", parsed.Key)
	assert.Equal(t, "1.0", parsed.Version)
}
