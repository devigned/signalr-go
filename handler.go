package signalr

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
)

type (
	// Handler is the default handler implementation for receiving SignalR invocations
	Handler interface {
		Default(ctx context.Context, target string, args []json.RawMessage) error
	}

	// HandlerFunc is a type converter that allows a func to be used as a `Handler`
	HandlerFunc func(ctx context.Context, target string, args []json.RawMessage) error

	// NotifiedHandler is a message handler which also provides an OnStart hook
	NotifiedHandler interface {
		Handler
		OnStart()
	}

	defaultNotifiedHandler struct {
		Handler
		onStart func()
	}
)

// Default redirects this call to the func that was provided
func (hf HandlerFunc) Default(ctx context.Context, target string, args []json.RawMessage) error {
	return hf(ctx, target, args)
}

// NewNotifiedHandler creates a new `NotifiedHandler` for responding to SignalR invocations
func NewNotifiedHandler(base Handler, onStart func()) NotifiedHandler {
	return &defaultNotifiedHandler{
		Handler: base,
		onStart: onStart,
	}
}

// OnStart calls the onStart function provided in the `NewNotifiedHandler` func when the client is done negotiating the
// initial handshake with the SignalR service
func (nh defaultNotifiedHandler) OnStart() {
	nh.onStart()
}

func dispatch(ctx context.Context, handler Handler, msg *InvocationMessage) error {
	t := reflect.TypeOf(handler)
	if method, ok := t.MethodByName(msg.Target); ok {
		mt := method.Type
		numIn := mt.NumIn()
		if numIn == len(msg.Arguments)+2 && method.Name != "Default" { // account for instance + context + arguments
			args := make([]reflect.Value, mt.NumIn()-1)
			args[0] = reflect.ValueOf(ctx)
			for i := 0; i < len(msg.Arguments); i++ {
				argType := mt.In(i + 2)
				newArg := reflect.New(argType)
				newArgPtr := newArg.Interface()
				err := json.Unmarshal(msg.Arguments[i], &newArgPtr)
				if err != nil {
					fmt.Println(err)
					return handler.Default(ctx, msg.Target, msg.Arguments)
				}

				args[i+1] = newArg.Elem()
			}

			actualTarget := reflect.ValueOf(handler).MethodByName(msg.Target)
			returns := actualTarget.Call(args)
			if len(returns) == 0 {
				return nil
			}

			if returns[0].IsNil() {
				return nil
			}

			if val, ok := returns[0].Elem().Interface().(error); ok {
				return val
			}

			fmt.Printf("ran into a return we didn't know how to deal with: %+v", returns)
			return nil
		}
	}

	return handler.Default(ctx, msg.Target, msg.Arguments)
}
