// Package tplink provides a Go client for the TP-Link Kasa Cloud API.
//
// The cloud API uses one POST endpoint for all operations. The method field in
// the JSON request body selects login, device listing, or passthrough. Device
// commands are JSON strings nested in passthrough requests. Account credentials
// are supplied on each request through AuthContext.
package tplink

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/portpowered/go-tplink/pkg/dependencies/cloud"
	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

var (
	errNilHTTPClient  = errors.New("HTTP client must not be nil")
	errInvalidBaseURL = errors.New(
		"base URL must be an HTTP(S) URL without credentials, query, or fragment",
	)
)

// HTTPDoer is the part of net/http.Client used by Client.
//
// Implementations shared by multiple goroutines must support concurrent calls.
type HTTPDoer = cloud.HTTPDoer

// Client is the TP-Link Cloud API client.
type Client struct {
	httpClient HTTPDoer
	baseURL    *url.URL

	mu     sync.RWMutex
	closed bool
}

// Option configures a Client.
type Option interface {
	Apply(client *Client) error
}

// NewClient creates a TP-Link Cloud API client with optional transport and
// regional endpoint overrides.
func NewClient(options ...Option) (*Client, error) {
	baseURL, err := parseBaseURL(DefaultBaseURL)
	if err != nil {
		return nil, tplinkmodels.NewConfigurationError("invalid default base URL", err)
	}

	client := new(Client)
	client.httpClient = http.DefaultClient
	client.baseURL = baseURL

	for _, option := range options {
		if option == nil {
			return nil, tplinkmodels.NewConfigurationError("client option must not be nil", nil)
		}

		err := option.Apply(client)
		if err != nil {
			return nil, tplinkmodels.NewConfigurationError("failed to apply client option", err)
		}
	}

	return client, nil
}

// Close prevents new requests from being sent by the client.
func (client *Client) Close() error {
	client.mu.Lock()
	defer client.mu.Unlock()

	client.closed = true

	return nil
}

type withHTTPClient struct{ client HTTPDoer }

func (option withHTTPClient) Apply(client *Client) error {
	if isNilHTTPDoer(option.client) {
		return errNilHTTPClient
	}

	client.httpClient = option.client

	return nil
}

func isNilHTTPDoer(client HTTPDoer) bool {
	if client == nil {
		return true
	}

	value := reflect.ValueOf(client)
	switch value.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return value.IsNil()
	case reflect.Invalid, reflect.Bool, reflect.Int, reflect.Int8, reflect.Int16,
		reflect.Int32, reflect.Int64, reflect.Uint, reflect.Uint8, reflect.Uint16,
		reflect.Uint32, reflect.Uint64, reflect.Uintptr, reflect.Float32,
		reflect.Float64, reflect.Complex64, reflect.Complex128, reflect.Array,
		reflect.String, reflect.Struct, reflect.UnsafePointer:
		return false
	}

	return false
}

// WithHTTPClient sets an HTTP implementation, such as *http.Client or a test
// transport that implements HTTPDoer.
//
//nolint:ireturn // Client options compose through the public Option interface.
func WithHTTPClient(client HTTPDoer) Option {
	return withHTTPClient{client: client}
}

type withBaseURL string

func (option withBaseURL) Apply(client *Client) error {
	baseURL, err := parseBaseURL(string(option))
	if err != nil {
		return err
	}

	client.baseURL = baseURL

	return nil
}

// WithBaseURL overrides the regional API endpoint. The URL must use HTTP or
// HTTPS and must not contain credentials, a query, or a fragment.
//
//nolint:ireturn // Client options compose through the public Option interface.
func WithBaseURL(value string) Option {
	return withBaseURL(value)
}

func parseBaseURL(value string) (*url.URL, error) {
	baseURL, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}

	if (baseURL.Scheme != "https" && baseURL.Scheme != "http") ||
		baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" ||
		baseURL.Fragment != "" {
		return nil, errInvalidBaseURL
	}

	return baseURL, nil
}

func (client *Client) doCloudRequest(
	ctx context.Context,
	operation string,
	cloudRequest any,
	auth *AuthContext,
) ([]byte, error) {
	if client == nil {
		return nil, tplinkmodels.NewConfigurationError("client is not initialized", nil)
	}

	if ctx == nil {
		return nil, tplinkmodels.NewInvalidRequestError("context must not be nil", nil)
	}

	client.mu.RLock()
	closed := client.closed
	client.mu.RUnlock()

	if closed {
		return nil, &tplinkmodels.ClientClosedError{}
	}

	if client.httpClient == nil || client.baseURL == nil {
		return nil, tplinkmodels.NewConfigurationError("client is not initialized", nil)
	}

	var token *string

	if auth != nil {
		if auth.AccessToken == "" {
			return nil, tplinkmodels.NewTokenNotSetError("no token supplied for this request")
		}

		token = &auth.AccessToken
	}

	response, err := cloud.Send(ctx, client.httpClient, client.baseURL, operation, cloudRequest, token)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return response, nil
}
func (client *Client) doPassthrough(
	ctx context.Context,
	operation string,
	auth AuthContext,
	deviceID string,
	command any,
) ([]byte, error) {
	commandBytes, err := json.Marshal(command)
	if err != nil {
		return nil, tplinkmodels.NewInvalidRequestError("failed to marshal device command", err)
	}

	cloudRequest := dependencymodels.PassthroughCloudRequest{
		Method: dependencymodels.MethodPassthrough,
		Params: dependencymodels.PassthroughParams{
			DeviceId:    deviceID,
			RequestData: string(commandBytes),
		},
	}

	responseBytes, err := client.doCloudRequest(ctx, operation, cloudRequest, &auth)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	err = cloud.CheckError(responseBytes, operation)
	if err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	var response dependencymodels.PassthroughResponse

	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough response", err)
	}

	responseData := ""
	if response.Result != nil {
		responseData = valueOrZero(response.Result.ResponseData)
	}

	return []byte(responseData), nil
}

func checkDeviceError(data []byte) error {
	var response dependencymodels.DeviceCommandError

	err := json.Unmarshal(data, &response)

	if err == nil && response.ErrCode != dependencymodels.DeviceErrorOK {
		if response.ErrCode == dependencymodels.DeviceErrorUnsupported {
			return tplinkmodels.NewUnsupportedOperationError(valueOrZero(response.ErrMsg))
		}

		return tplinkmodels.NewDeviceError(response.ErrCode, valueOrZero(response.ErrMsg))
	}

	return nil
}

//nolint:ireturn // This generic helper returns the caller's exact value type.
func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T

		return zero
	}

	return *value
}
