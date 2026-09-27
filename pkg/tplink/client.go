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

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const maxResponseBytes int64 = 1 << 20

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
	client := &Client{
		httpClient: http.DefaultClient,
		baseURL:    baseURL,
	}
	for _, option := range options {
		if option == nil {
			return nil, tplinkmodels.NewConfigurationError("client option must not be nil", nil)
		}
		if err := option.Apply(client); err != nil {
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
		return fmt.Errorf("HTTP client must not be nil")
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
	default:
		return false
	}
}

// WithHTTPClient sets an HTTP implementation, such as *http.Client or a test
// transport that implements HTTPDoer.
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
func WithBaseURL(value string) Option {
	return withBaseURL(value)
}

func parseBaseURL(value string) (*url.URL, error) {
	baseURL, err := url.Parse(strings.TrimSpace(value))
	if err != nil {
		return nil, err
	}
	if (baseURL.Scheme != "https" && baseURL.Scheme != "http") ||
		baseURL.Host == "" || baseURL.User != nil || baseURL.RawQuery != "" ||
		baseURL.Fragment != "" {
		return nil, fmt.Errorf("base URL must be an HTTP(S) URL without credentials, query, or fragment")
	}
	return baseURL, nil
}

func (client *Client) doCloudRequest(
	ctx context.Context,
	operation string,
	cloudRequest tplinkmodels.CloudRequest,
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
	cloudRequest tplinkmodels.CloudRequest,
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
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return nil, tplinkmodels.NewNetworkError("request failed", redactTransportError(err))
	}
	if response == nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "HTTP client returned a nil response", nil)
	}
	if response.Body != nil {
		defer func() { _ = response.Body.Close() }()
	}

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
	var response tplinkmodels.CloudResponse
	if err := json.Unmarshal(data, &response); err != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse cloud response", err)
	}
	if response.ErrorCode == 0 {
		return nil
	}

	switch response.ErrorCode {
	case -20004:
		return tplinkmodels.NewRateLimitError(response.Msg)
	case -20651:
		return tplinkmodels.NewTokenExpiredError(response.Msg)
	case -20104:
		return tplinkmodels.NewParameterError(response.Msg)
	case -20601:
		return tplinkmodels.NewAuthenticationError(response.Msg, response.ErrorCode)
	default:
		return tplinkmodels.NewCloudAPIError(response.ErrorCode, response.Msg)
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
	cloudRequest := tplinkmodels.CloudRequest{
		Method: MethodPassthrough,
		Params: tplinkmodels.PassthroughParams{
			DeviceID:    deviceID,
			RequestData: string(commandBytes),
		},
	}
	responseBytes, err := client.doCloudRequest(ctx, operation, cloudRequest, &auth)
	if err != nil {
		return nil, err
	}
	if err := checkCloudError(responseBytes, operation); err != nil {
		return nil, err
	}

	var response tplinkmodels.PassthroughResponse
	if err := json.Unmarshal(responseBytes, &response); err != nil {
		return nil, tplinkmodels.NewInvalidResponseError(operation, "failed to parse passthrough response", err)
	}
	return []byte(response.Result.ResponseData), nil
}

func checkDeviceError(data []byte) error {
	var response struct {
		ErrCode int    `json:"err_code"`
		ErrMsg  string `json:"err_msg"`
	}
	if err := json.Unmarshal(data, &response); err == nil && response.ErrCode != 0 {
		if response.ErrCode == -1 {
			return tplinkmodels.NewUnsupportedOperationError(response.ErrMsg)
		}
		return tplinkmodels.NewDeviceError(response.ErrCode, response.ErrMsg)
	}
	return nil
}
