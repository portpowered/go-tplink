// Package cloud contains HTTP transport and cloud-envelope handling for TP-Link requests.
package cloud

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const maxResponseBytes int64 = 1 << 20

// HTTPDoer is the request execution interface used by Client.
type HTTPDoer interface {
	Do(request *http.Request) (*http.Response, error)
}

// Send marshals one cloud request, executes it, and returns its successful response body.
func Send(
	ctx context.Context,
	httpClient HTTPDoer,
	baseURL *url.URL,
	operation string,
	cloudRequest any,
	token *string,
) ([]byte, error) {
	if ctx == nil {
		return nil, tplinkmodels.NewInvalidRequestError("context must not be nil", nil)
	}

	if httpClient == nil || baseURL == nil {
		return nil, tplinkmodels.NewConfigurationError("client is not initialized", nil)
	}

	request, err := newRequest(ctx, baseURL, cloudRequest, token)
	if err != nil {
		return nil, err
	}

	response, err := httpClient.Do(request)
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

	return readResponse(operation, response)
}

func newRequest(ctx context.Context, baseURL *url.URL, cloudRequest any, token *string) (*http.Request, error) {
	bodyBytes, err := json.Marshal(cloudRequest)
	if err != nil {
		return nil, tplinkmodels.NewInvalidRequestError("failed to marshal request", err)
	}

	if token != nil && *token == "" {
		return nil, tplinkmodels.NewTokenNotSetError("no token supplied for this request")
	}

	requestURL := requestURL(baseURL, token)

	request, err := http.NewRequestWithContext(
		ctx,
		dependencymodels.CloudRequestHTTPMethod,
		requestURL.String(),
		bytes.NewReader(bodyBytes),
	)
	if err != nil {
		return nil, tplinkmodels.NewInvalidRequestError("failed to create HTTP request", err)
	}

	request.Header.Set(dependencymodels.CloudRequestContentTypeHeader, dependencymodels.CloudRequestContentType)

	return request, nil
}

func requestURL(baseURL *url.URL, token *string) *url.URL {
	result := *baseURL
	if result.Path == "" && len(dependencymodels.CloudRequestPath) > 1 {
		result.Path = dependencymodels.CloudRequestPath
	}

	if token != nil {
		query := result.Query()
		query.Set(dependencymodels.TokenQueryKey, *token)
		result.RawQuery = query.Encode()
	}

	return &result
}

// CheckError converts a non-zero provider cloud code into the public SDK error type.
func CheckError(data []byte, operation string) error {
	var response dependencymodels.CloudResponse

	err := json.Unmarshal(data, &response)
	if err != nil {
		return tplinkmodels.NewInvalidResponseError(operation, "failed to parse cloud response", err)
	}

	errorCode := valueOrZero(response.ErrorCode)
	if errorCode == dependencymodels.CloudErrorCodeOK {
		return nil
	}

	message := valueOrZero(response.Msg)

	switch errorCode {
	case dependencymodels.CloudErrorCodeRateLimited:
		return tplinkmodels.NewRateLimitError(message)
	case dependencymodels.CloudErrorCodeTokenExpired:
		return tplinkmodels.NewTokenExpiredError(message)
	case dependencymodels.CloudErrorCodeParameter:
		return tplinkmodels.NewParameterError(message)
	case dependencymodels.CloudErrorCodeAuthentication:
		return tplinkmodels.NewAuthenticationError(message, errorCode)
	default:
		return tplinkmodels.NewCloudAPIError(errorCode, message)
	}
}

func readResponse(operation string, response *http.Response) ([]byte, error) {
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

func closeResponseBody(response *http.Response) {
	if response != nil && response.Body != nil {
		_ = response.Body.Close()
	}
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
		prefix, _, found := strings.Cut(value, "?")
		if found {
			return prefix + "?[REDACTED]"
		}

		return value
	}

	query := parsed.Query()
	if query.Has(dependencymodels.TokenQueryKey) {
		query.Set(dependencymodels.TokenQueryKey, "[REDACTED]")
		parsed.RawQuery = query.Encode()
	}

	return parsed.String()
}

//nolint:ireturn // This generic helper returns the caller's exact value type.
func valueOrZero[T any](value *T) T {
	if value == nil {
		var zero T

		return zero
	}

	return *value
}
