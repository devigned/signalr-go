package signalr_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/alexsasharegan/dotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/devigned/signalr-go"
)

func init() {
	if err := dotenv.Load(); err != nil {
		fmt.Println("Failed to load the .env file. If you expected env vars to be loaded from there, they weren't.")
	}
}

func TestClient_Listen(t *testing.T) {
	client := buildClient(t)

	handler := signalr.HandlerFunc(func(ctx context.Context, target string, args []json.RawMessage) error {
		fmt.Println(target, args)
		return nil
	})

	err := client.Listen(context.Background(), handler)
	assert.NoError(t, err)
}

func TestClient_Broadcast(t *testing.T) {
	withContext(func(ctx context.Context){
		client := buildClient(t)
		msg, err := signalr.NewInvocationMessage("foo")
		assert.NoError(t, err)
		assert.NoError(t, client.Broadcast(ctx, msg))
	})
}

func buildClient(t *testing.T) *signalr.Client {
	client, err := signalr.NewClient(os.Getenv("SIGNALR_CONNECTION_STRING"), "foo")
	if err != nil {
		require.NoError(t, err)
	}
	return client
}

func withContext(test func(ctx context.Context)) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	test(ctx)
}