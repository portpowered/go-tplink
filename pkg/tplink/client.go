// Package tplink provides a Go client for the TP-Link Kasa Cloud API.
//
// The cloud API uses one POST endpoint for all operations. The method field in
// the JSON request body selects login, device listing, or passthrough. Device
// commands are JSON strings nested in passthrough requests. Account credentials
// are supplied on each request through AuthContext.
package tplink

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"reflect"
	"strings"
	"sync"

	"github.com/portpowered/go-tplink/pkg/generatedwire"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const maxResponseBytes int64 = 1 << 20

var (
	errNilHTTPClient  = errors.New("HTTP client must not be nil")
	errInvalidBaseURL = errors.New(
		"base URL must be an HTTP(S) URL without credentials, query, or fragment",
	)
)

// HTTPDoer is the part of net/http.Client used by Client.
//
// Implementations shared by multiple goroutines must support concurrent calls.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

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
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
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
	request, err := client.newCloudHTTPRequest(ctx, cloudRequest, auth)
	if err != nil {
		return nil, err
	}

	return client.sendCloudRequest(operation, request)
}

func (client *Client) newCloudHTTPRequest(
	ctx context.Context,
	cloudRequest any,
	auth *AuthContext,
) (*http.Request, error) {
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

	bodyBytes, err := json.Marshal(cloudRequest)
	if err != nil {
		return nil, tplinkmodels.NewInvalidRequestError("failed to marshal request", err)
	}

	requestURL := *client.baseURL

	if auth != nil {
		if auth.AccessToken == "" {
			return nil, tplinkmodels.NewTokenNotSetError("no token supplied for this request")
		}

		query := requestURL.Query()
		query.Set("token", auth.AccessToken)
		requestURL.RawQuery = query.Encode()
	}

	request, err := http.NewRequestWithContext(ctx, http.MethodPost, requestURL.String(), bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, tplinkmodels.NewInvalidRequestError("failed to create HTTP request", err)
	}

	request.Header.Set("Content-Type", "application/json")

	return request, nil
}

func (client *Client) sendCloudRequest(operation string, request *http.Request) ([]byte, error) {
	response, err := client.httpClient.Do(request)
	if err != nil {
		closeResponseBody(response)

		return nil, tplinkmodels.NewNetworkError("request failed", redactTransportError(err))
	}

	if response == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "HTTP client returned a nil response", nil)
	}

	if response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}

	return readCloudResponse(operation, response)
}

func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
}

func readCloudResponse(operation string, response *http.Response) ([]byte, error) {
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		return nil, &tplinkmodels.HTTPStatusError{
			Operation:  operation,
			StatusCode: response.StatusCode,
		}
	}

	if response.Body == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "HTTP response body is nil", nil)
	}

	responseBytes, err := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes+1))
	if err != nil {
		return nil, tplinkmodels.NewNetworkError("failed to read response body", err)
	}

	if int64(len(responseBytes)) > maxResponseBytes {
		return nil, &tplinkmodels.ResponseTooLargeError{Operation: operation, Limit: maxResponseBytes}
	}

	return responseBytes, nil
}

func redactTransportError(err error) error {
	var requestError *url.Error
	if !errors.As(err, &requestError) {
		return err
	}

	return &url.Error{
		Op:  requestError.Op,
		URL: redactURL(requestError.URL),
		Err: requestError.Err,
	}
}

func redactURL(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		if queryStart := strings.IndexByte(value, '?'); queryStart >= 0 {
			return value[:queryStart] + "?[REDACTED]"
		}

		return value
	}

	query := parsed.Query()
	if query.Has("token") {
		query.Set("token", "[REDACTED]")
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}

func checkCloudError(data []byte, operation string) error {
	var response generatedwire.CloudResponse

	err := json.Unmarshal(data, &response)
	if err != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse cloud response", err)
	}

	errorCode := valueOrZero(response.ErrorCode)
	if errorCode == 0 {
		return nil
	}

	message := valueOrZero(response.Msg)

	switch errorCode {
	case -20004:
		return tplinkmodels.NewRateLimitError(message)
	case -20651:
		return tplinkmodels.NewTokenExpiredError(message)
	case -20104:
		return tplinkmodels.NewParameterError(message)
	case -20601:
		return tplinkmodels.NewAuthenticationError(message, errorCode)
	default:
		return tplinkmodels.NewCloudAPIError(errorCode, message)
	}
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

	cloudRequest := generatedwire.PassthroughCloudRequest{
		Method: generatedwire.Passthrough,
		Params: generatedwire.PassthroughParams{
			DeviceId:    deviceID,
			RequestData: string(commandBytes),
		},
	}

	responseBytes, err := client.doCloudRequest(ctx, operation, cloudRequest, &auth)
	if err != nil {
		return nil, err
	}

	err = checkCloudError(responseBytes, operation)
	if err != nil {
		return nil, err
	}

	var response generatedwire.PassthroughResponse

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
	var response generatedwire.DeviceCommandError

	err := json.Unmarshal(data, &response)

	if err == nil && response.ErrCode != 0 {
		if response.ErrCode == -1 {
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
