package signalr_test

import (
	"context"
	"fmt"
	"os"

	"github.com/devigned/signalr-go"
)

func ExampleNewClient() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := "chat"
	client, err := signalr.NewClient(connStr, hubName)
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(client.GetHub())

	// Output: chat
}

func ExampleClientWithName() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	clientName := "myName123"
	client, err := signalr.NewClient(connStr, hubName, signalr.ClientWithName(clientName))
	if err != nil {
		fmt.Println(err)
		return
	}

	fmt.Println(client.GetName()) // client can be addressed directly from this name

	// Output: myName123
}

func ExampleClient_Listen() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	client, err := signalr.NewClient(connStr, hubName)
	if err != nil {
		fmt.Println(err)
		return
	}

	listenCtx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{}, 1)
	mph := &MessagePrinterHandler{
		onStart: func() {
			fmt.Println("I'm listening")
			started <- struct{}{}
			cancel()
		},
	}
	// since `MessagePrinterHandler` implements `NotifiedHandler` it will receive a call to `OnStart` when listening

	go func() {
		err = client.Listen(listenCtx, mph)
		if err != nil {
			fmt.Println(err)
			started <- struct{}{}
		}
	}()
	<-started

	// Output: I'm listening
}

func ExampleClient_SendToUser() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	clientName := randomName("client", 5)
	client, err := signalr.NewClient(connStr, hubName, signalr.ClientWithName(clientName))
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{}, 1)
	mph := &MessagePrinterHandler{
		onStart: func() {
			started <- struct{}{}
		},
		cancel: cancel,
	}

	go func() {
		err = client.Listen(ctx, mph)
		if err != nil {
			fmt.Println(err)
			started <- struct{}{}
		}
	}()
	<-started

	msg, err := signalr.NewInvocationMessage("Println", "hello only to you")
	if err != nil {
		fmt.Println(err)
		return
	}

	err = client.SendToUser(ctx, msg, clientName)
	if err != nil {
		fmt.Println(err)
		return
	}

	// wait for the `MessagePrinterHandler` to call cancel on the context
	<-ctx.Done()

	// Output: hello only to you
}

func ExampleClient_BroadcastAll() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	client, err := signalr.NewClient(connStr, hubName)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())

	started := make(chan struct{}, 1)
	mph := &MessagePrinterHandler{
		onStart: func() {
			started <- struct{}{}
		},
		cancel: cancel,
	}

	go func() {
		err = client.Listen(ctx, mph)
		if err != nil {
			fmt.Println(err)
			started <- struct{}{}
		}
	}()
	<-started

	msg, err := signalr.NewInvocationMessage("Println", "hello world!")
	if err != nil {
		fmt.Println(err)
		return
	}

	err = client.BroadcastAll(ctx, msg)
	if err != nil {
		fmt.Println(err)
		return
	}

	// wait for the `MessagePrinterHandler` to call cancel on the context
	<-ctx.Done()

	// Output: hello world!
}

func ExampleClient_BroadcastGroup() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	groupName := randomName("group", 5)
	client, err := signalr.NewClient(connStr, hubName)
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := client.AddUserToGroup(ctx, groupName, client.GetName()); err != nil {
		fmt.Println(err)
		return
	}

	started := make(chan struct{}, 1)
	mph := &MessagePrinterHandler{
		onStart: func() {
			started <- struct{}{}
		},
		cancel: cancel,
	}

	go func() {
		err = client.Listen(ctx, mph)
		if err != nil {
			fmt.Println(err)
			started <- struct{}{}
		}
	}()
	<-started

	msg, err := signalr.NewInvocationMessage("Println", "I'm part of the cool group")
	if err != nil {
		fmt.Println(err)
		return
	}

	err = client.BroadcastGroup(ctx, msg, groupName)
	if err != nil {
		fmt.Println(err)
		return
	}

	// wait for the `MessagePrinterHandler` to call cancel on the context
	<-ctx.Done()

	// Output: I'm part of the cool group
}

func ExampleClient_AddUserToGroup() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	groupName := "someGroup"
	clientName := "client42"
	client, err := signalr.NewClient(connStr, hubName, signalr.ClientWithName(clientName))
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.AddUserToGroup(ctx, groupName, client.GetName()); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("added user %q to group %q\n", clientName, groupName)

	// Output: added user "client42" to group "someGroup"
}

func ExampleClient_RemoveUserFromAllGroups() {
	connStr := os.Getenv("SIGNALR_CONNECTION_STRING")
	hubName := randomName("example", 5)
	clientName := "client42"
	client, err := signalr.NewClient(connStr, hubName, signalr.ClientWithName(clientName))
	if err != nil {
		fmt.Println(err)
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := client.RemoveUserFromAllGroups(ctx, client.GetName()); err != nil {
		fmt.Println(err)
		return
	}

	fmt.Printf("removed user %q from all groups\n", clientName)

	// Output: removed user "client42" from all groups
}
