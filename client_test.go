package signalr_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/alexsasharegan/dotenv"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/devigned/signalr-go"
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

func init() {
	if err := dotenv.Load(); err != nil {
		fmt.Println("Failed to load the .env file. If you expected env vars to be loaded from there, they weren't.")
	}
	rand.Seed(time.Now().Unix())
}

func TestClient_Listen(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		listenCtx, cancel := context.WithCancel(ctx)
		started := make(chan struct{}, 1)
		var targetName string

		go func() {
			cancelingHandler := signalr.NewNotifiedHandler(
				signalr.HandlerFunc(func(ctx context.Context, target string, args []json.RawMessage) error {
					targetName = target
					cancel()
					return nil
				}), func() {
					started <- struct{}{}
				})

			err := client.Listen(listenCtx, cancelingHandler)
			assert.NoError(t, err) // should not have an error since the context is cancelled and we stop listening
		}()

		select {
		case <-started:
		case <-ctx.Done():
		}

		msg, err := signalr.NewInvocationMessage("foo")
		require.NoError(t, err)
		require.NoError(t, client.Broadcast(ctx, msg)) // broadcast a message to all clients
		<-listenCtx.Done()
		assert.Equal(t, "foo", targetName)
	})
}

func TestClient_Broadcast(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		msg, err := signalr.NewInvocationMessage("foo")
		assert.NoError(t, err)
		assert.NoError(t, client.Broadcast(ctx, msg))
	})
}

func buildClient(t *testing.T, hubName string) *signalr.Client {
	client, err := signalr.NewClient(os.Getenv("SIGNALR_CONNECTION_STRING"), hubName)
	if err != nil {
		require.NoError(t, err)
	}
	return client
}

func withContext(t *testing.T, test func(ctx context.Context, client *signalr.Client)) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hubName := randomName("gotest_", 10)
	client := buildClient(t, hubName)
	test(ctx, client)
}

func randomName(prefix string, length int) string {
	return randomString(prefix, length)
}

func randomString(prefix string, length int) string {
	b := make([]rune, length)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return prefix + string(b)
}
