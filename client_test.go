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
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/devigned/signalr-go"
)

type (
	FancyHandlerMock struct {
		mock.Mock
		onStart func()
		cancel  func()
	}

	ComplexObject struct {
		FieldString string `json:"fieldString,omitempty"`
		FieldInt    int    `json:"fieldInt,omitempty"`
	}
)

var (
	letterRunes = []rune("abcdefghijklmnopqrstuvwxyz123456789")
)

func (fhm *FancyHandlerMock) Default(ctx context.Context, target string, args []json.RawMessage) error {
	defer fhm.cancel()
	it := fhm.Called(ctx, target, args)
	return it.Error(0)
}

func (fhm *FancyHandlerMock) TargetFunc(ctx context.Context, arg1 string, arg2 *ComplexObject) error {
	defer fhm.cancel()
	it := fhm.Called(ctx, arg1, arg2)
	return it.Error(0)
}

func (fhm *FancyHandlerMock) OnStart() {
	fhm.onStart()
}

func init() {
	if err := dotenv.Load(); err != nil {
		fmt.Println("Failed to load the .env file. If you expected env vars to be loaded from there, they weren't.")
	}
	rand.Seed(time.Now().Unix())
}

func TestClientWithName(t *testing.T) {
	name := "myClient"
	client, err := signalr.NewClient(
		os.Getenv("SIGNALR_CONNECTION_STRING"),
		"hub1",
		signalr.ClientWithName(name),
	)
	assert.NoError(t, err)
	assert.Equal(t, name, client.GetName())
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
		require.NoError(t, client.BroadcastAll(ctx, msg)) // broadcast a message to all clients
		<-listenCtx.Done()
		assert.Equal(t, "foo", targetName)
	})
}

func TestClient_ListenWithHandlerMethod(t *testing.T) {
	target := "TargetFunc"
	arg1 := "hello world!"
	arg2 := &ComplexObject{
		FieldString: "fieldString",
		FieldInt:    42,
	}
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		listenCtx, cancel := context.WithCancel(ctx)
		started := make(chan struct{}, 1)

		h := &FancyHandlerMock{
			onStart: func() {
				started <- struct{}{}
			},
			cancel: cancel,
		}

		h.On(target, mock.Anything, arg1, mock.MatchedBy(func(obj *ComplexObject) bool {
			return obj.FieldInt == arg2.FieldInt && obj.FieldString == obj.FieldString
		})).Return(nil).Once()

		go func() {
			err := client.Listen(listenCtx, h)
			assert.NoError(t, err)
		}()

		select {
		case <-started:
		case <-ctx.Done():
		}

		msg, err := signalr.NewInvocationMessage(target, arg1, arg2)
		require.NoError(t, err)
		require.NoError(t, client.BroadcastAll(ctx, msg)) // broadcast a message to all clients
		<-listenCtx.Done()
		h.AssertExpectations(t)
	})
}

func TestClient_SendToUserAndReceive(t *testing.T) {
	clientName := uuid.Must(uuid.NewRandom()).String()
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		listenCtx, cancel := context.WithCancel(ctx)
		started := make(chan struct{}, 1)

		h := &FancyHandlerMock{
			onStart: func() {
				started <- struct{}{}
			},
			cancel: cancel,
		}

		h.On("Default", mock.Anything, mock.MatchedBy(func(targetName string) bool {
			return targetName == "method2"
		}), mock.Anything).Return(nil).Once()

		go func() {
			err := client.Listen(listenCtx, h)
			assert.NoError(t, err)
		}()

		select {
		case <-started:
		case <-ctx.Done():
		}

		msg1, err := signalr.NewInvocationMessage("method1", "hello")
		require.NoError(t, err)
		msg2, err := signalr.NewInvocationMessage("method2", "helloFrom2")
		require.NoError(t, err)
		require.NoError(t, client.SendToUser(ctx, msg1, clientName + "blah")) // will never be received by the client
		require.NoError(t, client.SendToUser(ctx, msg2, clientName)) // send to a single user
		<-listenCtx.Done()
		h.AssertExpectations(t)
	}, signalr.ClientWithName(clientName))
}

func TestClient_SendToGroupAndReceive(t *testing.T) {
	clientName := uuid.Must(uuid.NewRandom()).String()
	groupName := "group" + uuid.Must(uuid.NewRandom()).String()
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		listenCtx, cancel := context.WithCancel(ctx)
		require.NoError(t, client.AddUserToGroup(ctx, groupName, client.GetName()))

		started := make(chan struct{}, 1)
		h := &FancyHandlerMock{
			onStart: func() {
				started <- struct{}{}
			},
			cancel: cancel,
		}

		h.On("Default", mock.Anything, mock.MatchedBy(func(targetName string) bool {
			return targetName == "method2"
		}), mock.Anything).Return(nil).Once()

		go func() {
			err := client.Listen(listenCtx, h)
			assert.NoError(t, err)
		}()

		select {
		case <-started:
		case <-ctx.Done():
		}

		msg1, err := signalr.NewInvocationMessage("method1", "hello")
		require.NoError(t, err)
		msg2, err := signalr.NewInvocationMessage("method2", "helloFrom2")
		require.NoError(t, err)
		require.NoError(t, client.BroadcastGroup(ctx, msg1, groupName + "blah")) // will never be received by the client
		require.NoError(t, client.BroadcastGroup(ctx, msg2, groupName)) // send to the group containing the client
		<-listenCtx.Done()
		h.AssertExpectations(t)
	}, signalr.ClientWithName(clientName))
}


func TestClient_Broadcast(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		msg, err := signalr.NewInvocationMessage("foo")
		assert.NoError(t, err)
		assert.NoError(t, client.BroadcastAll(ctx, msg))
	})
}

func TestClient_BroadcastGroup(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		msg, err := signalr.NewInvocationMessage("foo")
		assert.NoError(t, err)
		assert.NoError(t, client.BroadcastGroup(ctx, msg, "group1"))
	})
}

func TestClient_AddUserToGroup(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		assert.NoError(t, client.AddUserToGroup(ctx, "group1", "user1"))
	})
}

func TestClient_RemoveUserFromGroup(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		assert.NoError(t, client.RemoveUserFromGroup(ctx, "group1", "user1"))
	})
}

func TestClient_RemoveUserFromAllGroups(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		assert.NoError(t, client.RemoveUserFromAllGroups(ctx, "user1"))
	})
}

func TestClient_SendToUser(t *testing.T) {
	withContext(t, func(ctx context.Context, client *signalr.Client) {
		msg, err := signalr.NewInvocationMessage("foo")
		assert.NoError(t, err)
		assert.NoError(t, client.SendToUser(ctx, msg, "user1"))
	})
}

func buildClient(t *testing.T, hubName string, opts... signalr.ClientOption) *signalr.Client {
	client, err := signalr.NewClient(os.Getenv("SIGNALR_CONNECTION_STRING"), hubName, opts...)
	if err != nil {
		require.NoError(t, err)
	}
	return client
}

func withContext(t *testing.T, test func(ctx context.Context, client *signalr.Client), opts... signalr.ClientOption) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hubName := randomName("gotest_", 10)
	client := buildClient(t, hubName, opts...)
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
