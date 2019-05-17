package signalr

import (
	"context"
	"encoding/json"
)

type (
	// Handler is the default handler implementation for receiving SignalR messages
	Handler interface {
		Default(ctx context.Context, target string, args []json.RawMessage) error
	}

	// HandlerFunc is a type converter that allows a func to be used as a `Handler`
	HandlerFunc func(ctx context.Context, target string, args []json.RawMessage) error

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

func NewNotifiedHandler(base Handler, onStart func()) NotifiedHandler {
	return &defaultNotifiedHandler{
		Handler: base,
		onStart: onStart,
	}
}

func (nh defaultNotifiedHandler) OnStart() {
	nh.onStart()
}