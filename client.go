package signalr

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/google/uuid"
	"nhooyr.io/websocket"
)

type (
	// Client represents a bidirectional connection to Azure SignalR
	Client struct {
		name          string
		hubName       string
		audType       audienceType
		parsedConnStr *ParsedConnString
		nMutex        sync.RWMutex
		negotiateRes  *negotiateResponse
	}

	// ClientOption provides a way to configure a client at time of construction
	ClientOption func(*Client) error

	// NegotiateResponse is the structure to respond to a client for access to a hub resource
	negotiateResponse struct {
		ConnectionID        string      `json:"connectionId,omitempty"`
		AvailableTransports []transport `json:"availableTransports"`
	}

	transport struct {
		Name    string   `json:"transport,omitempty"`
		Formats []string `json:"transportFormats,omitempty"`
	}

	audienceType   string
	transportTypes string

	signalrCliams struct {
		jwt.StandardClaims
		NameID string `json:"nameid,omitempty"`
	}

	handshakeRequest struct {
		Protocol string `json:"protocol"`
		Version  int    `json:"version"`
	}

	handshakeResponse struct {
		Error string `json:"error,omitempty"`
	}

	messageType int

	// InvocationMessage is the structure expected for sending and receiving in the SignalR protocol
	InvocationMessage struct {
		Type         messageType       `json:"type,omitempty"`
		Headers      map[string]string `json:"headers,omitempty"`
		InvocationID string            `json:"invocationId,omitempty"`
		Target       string            `json:"target"`
		Arguments    []json.RawMessage `json:"arguments"`
		Error        string            `json:"error,omitempty"`
	}
)

const (
	messageTerminator byte = 0x1E

	invocationMessageType messageType = iota
	streamItemMessageType
	completionMessageType
	streamInvocationMessageType
	cancelInvocationMessageType
	pingMessageType
	closeMessageType
)

var (
	serverAudienceType audienceType = "server"
	clientAudienceType audienceType = "client"

	websocketTransportType transportTypes = "WebSockets"
)

// ClientWithName configures a SignalR client to use a specific name for addressing the client individually
func ClientWithName(name string) ClientOption {
	return func(client *Client) error {
		client.name = name
		return nil
	}
}

// NewClient constructs a new client given a set of construction options
func NewClient(connStr string, hubName string, opts ...ClientOption) (*Client, error) {
	parsed, err := ParseConnectionString(connStr)
	if err != nil {
		return nil, err
	}

	client := &Client{
		hubName:       hubName,
		parsedConnStr: parsed,
		audType:       clientAudienceType,
		name:          uuid.Must(uuid.NewRandom()).String(),
	}

	for _, opt := range opts {
		if err := opt(client); err != nil {
			return client, err
		}
	}

	return client, nil
}

// Listen will start the WebSocket connection for the client
func (c *Client) Listen(ctx context.Context, handler Handler) error {
	err := c.negotiateOnce(ctx)
	if err != nil {
		return err
	}

	audience := c.getWssAudience()
	token, err := c.generateToken(audience, 2*time.Hour)
	if err != nil {
		return err
	}

	conn, resp, err := websocket.Dial(ctx, c.getWssURI(), websocket.DialOptions{
		HTTPHeader: http.Header{
			"Authorization": []string{"Bearer " + token},
		},
		HTTPClient: newHTTPClient(),
	})
	if resp != nil {
		defer func() {
			_ = resp.Body.Close()
		}()
	}

	if err != nil {
		return err
	}

	err = c.handshake(ctx, conn)
	if err != nil {
		return err
	}

	select {
	case <-ctx.Done():
	default:
		if h, ok := handler.(NotifiedHandler); ok {
			h.OnStart()
		}

		for {
			bits, err := readConn(ctx, conn)
			if err != nil {
				if err.Error() == "failed to get reader: context canceled" {
					return nil
				}
				return err
			}

			var msg InvocationMessage
			err = json.Unmarshal(bits, &msg)
			if err != nil {
				return err
			}

			//fmt.Println(string(bits))
			switch msg.Type {
			case pingMessageType:
				// nop
			case invocationMessageType:
				return dispatch(ctx, handler, &msg)
			case streamInvocationMessageType, streamItemMessageType, cancelInvocationMessageType, completionMessageType:
				return errors.New("unhandled InvocationMessage type: " + string(msg.Type))
			case closeMessageType:
				return conn.Close(websocket.StatusNormalClosure, "received close message from SignalR service")
			}

		}
	}

	return nil
}

// BroadcastAll will send a broadcast `InvocationMessage` to all listening to the hub
func (c *Client) BroadcastAll(ctx context.Context, msg *InvocationMessage) error {
	return c.SendInvocation(ctx, c.getBroadcastURI(), msg)
}

// BroadcastGroup will send a broadcast `InvocationMessage` to all listening to the hub group
func (c *Client) BroadcastGroup(ctx context.Context, msg *InvocationMessage, groupName string) error {
	return c.SendInvocation(ctx, c.getSendToGroupURI(groupName), msg)
}

// SendToUser will send a `InvocationMessage` to a particular user
func (c *Client) SendToUser(ctx context.Context, msg *InvocationMessage, userID string) error {
	return c.SendInvocation(ctx, c.getSendToUserURI(userID), msg)
}

// AddUserToGroup will add a userID to a SignalR group
func (c *Client) AddUserToGroup(ctx context.Context, groupName string, userID string) error {
	req, err := http.NewRequest(http.MethodPut, c.getGroupUserURI(groupName, userID), nil)
	if err != nil {
		return err
	}

	res, err := c.do(ctx, req)
	defer closeRes(res)
	if err != nil {
		return err
	}

	bodyBits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode > 399 {
		return SendFailureError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBits),
		}
	}

	return nil
}

// RemoveUserFromGroup will remove a userID from a SignalR group
func (c *Client) RemoveUserFromGroup(ctx context.Context, groupName string, userID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getGroupUserURI(groupName, userID), nil)
	if err != nil {
		return err
	}

	res, err := c.do(ctx, req)
	defer closeRes(res)
	if err != nil {
		return err
	}

	bodyBits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode > 399 {
		return SendFailureError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBits),
		}
	}

	return nil
}

// RemoveUserFromAllGroups will remove a user from all groups
func (c *Client) RemoveUserFromAllGroups(ctx context.Context, userID string) error {
	req, err := http.NewRequest(http.MethodDelete, c.getUsersGroupsURI(userID), nil)
	if err != nil {
		return err
	}

	res, err := c.do(ctx, req)
	defer closeRes(res)
	if err != nil {
		return err
	}

	bodyBits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode > 399 {
		return SendFailureError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBits),
		}
	}

	return nil
}

// SendInvocation will send an `InvocationMessage` to the hub
func (c *Client) SendInvocation(ctx context.Context, uri string, msg *InvocationMessage) error {
	bits, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, uri, bytes.NewReader(bits))
	if err != nil {
		return err
	}

	token, err := c.generateToken(uri, 2*time.Hour)
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.WithContext(ctx)
	client := newHTTPClient()
	res, err := client.Do(req)
	defer closeRes(res)

	if err != nil {
		fmt.Println(err)
	}

	bodyBits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return err
	}

	if res.StatusCode > 399 {
		return SendFailureError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBits),
		}
	}

	return nil
}

// GetName returns the name of the client
func (c *Client) GetName() string {
	return c.name
}

// GetHub returns the name of the SignalR hub the client is targeting
func (c *Client) GetHub() string {
	return c.hubName
}

func (c *Client) do(ctx context.Context, req *http.Request) (*http.Response, error) {
	token, err := c.generateToken(req.URL.String(), 2*time.Hour)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.WithContext(ctx)
	client := newHTTPClient()
	return client.Do(req)
}

func readConn(ctx context.Context, conn *websocket.Conn) ([]byte, error) {
	readerCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	_, reader, err := conn.Reader(readerCtx)
	if err != nil {
		return nil, err
	}

	bits, err := ioutil.ReadAll(reader)
	if err != nil {
		return nil, err
	}

	if bits[len(bits)-1] == messageTerminator {
		bits = bits[0 : len(bits)-1]
	}

	return bits, nil
}

func (c *Client) handshake(ctx context.Context, conn *websocket.Conn) error {
	hsReq := handshakeRequest{
		Protocol: "json",
		Version:  1,
	}

	bits, err := json.Marshal(hsReq)
	if err != nil {
		return err
	}

	wrCloser, err := conn.Writer(ctx, websocket.MessageText)
	if err != nil {
		return err
	}

	_, err = wrCloser.Write(append(bits, messageTerminator))
	if err != nil {
		return err
	}

	if err := wrCloser.Close(); err != nil {
		return err
	}

	_, resp, err := conn.Reader(ctx)
	if err != nil {
		return err
	}

	bits, err = ioutil.ReadAll(resp)
	if err != nil {
		return err
	}

	if bits[len(bits)-1] == messageTerminator {
		bits = bits[0 : len(bits)-1]
	}

	var hsRes handshakeResponse
	if err := json.Unmarshal(bits, &hsRes); err != nil {
		return err
	}

	if hsRes.Error != "" {
		return errors.New(hsRes.Error)
	}
	return nil
}

func (c *Client) generateToken(audience string, expiresAfter time.Duration) (string, error) {
	now := time.Now().UTC()
	claims := signalrCliams{
		StandardClaims: jwt.StandardClaims{
			IssuedAt:  now.Unix(),
			NotBefore: now.Unix(),
			Audience:  audience,
			ExpiresAt: now.Add(expiresAfter).Unix(),
		},
		NameID: c.name,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(c.parsedConnStr.Key))
}

func (c *Client) getWssURI() string {
	wssBaseURI := strings.Replace(c.getWssAudience(), "https://", "wss://", 1)
	return wssBaseURI + "&id=" + c.negotiateRes.ConnectionID
}

func (c *Client) getWssAudience() string {
	return fmt.Sprintf("%s/%s/?hub=%s", c.parsedConnStr.Endpoint.String(), c.audType, strings.ToLower(c.hubName))
}

func (c *Client) getBroadcastURI() string {
	return c.getBaseURI()
}

func (c *Client) getSendToUserURI(userID string) string {
	return fmt.Sprintf("%s/users/%s", c.getBaseURI(), userID)
}

func (c *Client) getSendToGroupURI(groupName string) string {
	return fmt.Sprintf("%s/groups/%s", c.getBaseURI(), groupName)
}

func (c *Client) getGroupUserURI(groupName, userID string) string {
	return fmt.Sprintf("%s/groups/%s/users/%s", c.getBaseURI(), groupName, userID)
}

func (c *Client) getUsersGroupsURI(userID string) string {
	return fmt.Sprintf("%s/users/%s/groups", c.getBaseURI(), userID)
}

func (c *Client) getBaseURI() string {
	return fmt.Sprintf("%s/api/v1/hubs/%s", c.parsedConnStr.Endpoint, strings.ToLower(c.hubName))
}

func newHTTPClient() *http.Client {
	tr := &http.Transport{
		MaxIdleConnsPerHost: 10,
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
	}

	return &http.Client{
		Transport: tr,
	}
}

func (c *Client) negotiateOnce(ctx context.Context) error {
	c.nMutex.Lock()
	defer c.nMutex.Unlock()

	if c.negotiateRes == nil {
		res, err := c.negotiate(ctx)
		if err != nil {
			return err
		}

		found := false
		for _, txport := range res.AvailableTransports {
			if txport.Name == string(websocketTransportType) {
				found = true
			}
		}

		if !found {
			return errors.New("WebSockets transport is not supported by the service")
		}

		c.negotiateRes = res
	}

	return nil
}

func (c *Client) negotiate(ctx context.Context) (*negotiateResponse, error) {
	endpoint := c.parsedConnStr.Endpoint
	negotiateURI := fmt.Sprintf("%s/%s/%s", endpoint, c.audType, "negotiate?hub="+c.hubName)
	req, err := http.NewRequest(http.MethodPost, negotiateURI, nil)
	if err != nil {
		return nil, err
	}

	audience := c.getWssAudience()
	token, err := c.generateToken(audience, 2*time.Hour)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.WithContext(ctx)
	client := newHTTPClient()
	res, err := client.Do(req)
	defer closeRes(res)

	if err != nil {
		fmt.Println(err)
	}

	bodyBits, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if res.StatusCode > 399 {
		return nil, &SendFailureError{
			StatusCode: res.StatusCode,
			Body:       string(bodyBits),
		}
	}

	var negRes negotiateResponse
	err = json.Unmarshal(bodyBits, &negRes)
	if err != nil {
		return nil, err
	}

	return &negRes, nil
}

// NewInvocationMessage creates a new `InvocationMessage` from a target method name and arguments
func NewInvocationMessage(target string, args ...interface{}) (*InvocationMessage, error) {
	jsonArgs := make([]json.RawMessage, len(args))
	for i := 0; i < len(args); i++ {
		bits, err := json.Marshal(args[i])
		if err != nil {
			return nil, err
		}
		jsonArgs[i] = json.RawMessage(bits)
	}

	return &InvocationMessage{
		Type:      invocationMessageType,
		Target:    target,
		Arguments: jsonArgs,
	}, nil
}

func closeRes(res *http.Response) {
	if res != nil {
		_ = res.Body.Close()
	}
}
