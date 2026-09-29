package tplinkmodels

import (
	"errors"
	"fmt"
)

// ConfigurationError indicates the client could not be configured.
type ConfigurationError struct {
	Message string
	Err     error
}

// NewConfigurationError creates a configuration error.
func NewConfigurationError(message string, err error) *ConfigurationError {
	return &ConfigurationError{Message: message, Err: err}
}

func (e *ConfigurationError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("configuration error: %s: %v", e.Message, e.Err)
	}

	return "configuration error: " + e.Message
}

func (e *ConfigurationError) Unwrap() error { return e.Err }

// IsConfigurationError reports whether err is a configuration error.
func IsConfigurationError(err error) bool { return isError[*ConfigurationError](err) }

// InvalidRequestError indicates the caller supplied an invalid request.
type InvalidRequestError struct {
	Message string
	Err     error
}

// NewInvalidRequestError creates an invalid request error.
func NewInvalidRequestError(message string, err error) *InvalidRequestError {
	return &InvalidRequestError{Message: message, Err: err}
}

func (e *InvalidRequestError) Error() string {
	if e.Err != nil {
		return fmt.Sprintf("invalid request: %s: %v", e.Message, e.Err)
	}

	return "invalid request: " + e.Message
}

func (e *InvalidRequestError) Unwrap() error { return e.Err }

// IsInvalidRequestError reports whether err is an invalid request error.
func IsInvalidRequestError(err error) bool { return isError[*InvalidRequestError](err) }

// HTTPStatusError indicates the provider rejected a request at the HTTP layer.
type HTTPStatusError struct {
	Operation  string
	StatusCode int
}

func (e *HTTPStatusError) Error() string {
	if e.Operation == "" {
		return fmt.Sprintf("provider returned HTTP status %d", e.StatusCode)
	}

	return fmt.Sprintf("%s: provider returned HTTP status %d", e.Operation, e.StatusCode)
}

// IsHTTPStatusError reports whether err is an HTTP status error.
func IsHTTPStatusError(err error) bool { return isError[*HTTPStatusError](err) }

// InvalidResponseError indicates that the provider returned an unusable response.
type InvalidResponseError struct {
	Operation string
	Message   string
	Err       error
}

// NewInvalidResponseError creates an invalid response error.
func NewInvalidResponseError(operation, message string, err error) *InvalidResponseError {
	return &InvalidResponseError{Operation: operation, Message: message, Err: err}
}

func (e *InvalidResponseError) Error() string {
	message := "invalid provider response"
	if e.Message != "" {
		message += ": " + e.Message
	}

	if e.Err != nil {
		message += ": " + e.Err.Error()
	}

	if e.Operation != "" {
		return e.Operation + ": " + message
	}

	return message
}

func (e *InvalidResponseError) Unwrap() error { return e.Err }

// IsInvalidResponseError reports whether err is an invalid response error.
func IsInvalidResponseError(err error) bool { return isError[*InvalidResponseError](err) }

// ResponseTooLargeError indicates that a provider response exceeded the client limit.
type ResponseTooLargeError struct {
	Operation string
	Limit     int64
}

func (e *ResponseTooLargeError) Error() string {
	if e.Operation != "" {
		return fmt.Sprintf("%s: provider response exceeds %d bytes", e.Operation, e.Limit)
	}

	return fmt.Sprintf("provider response exceeds %d bytes", e.Limit)
}

// IsResponseTooLargeError reports whether err is a response-too-large error.
func IsResponseTooLargeError(err error) bool { return isError[*ResponseTooLargeError](err) }

// ClientClosedError indicates that an operation was attempted after Close.
type ClientClosedError struct{}

func (*ClientClosedError) Error() string { return "client is closed" }

// IsClientClosedError reports whether err is a client-closed error.
func IsClientClosedError(err error) bool { return isError[*ClientClosedError](err) }

// RateLimitError indicates the TP-Link Cloud API rejected the request
// due to rate limiting (error_code -20004).
type RateLimitError struct {
	Message string
}

// NewRateLimitError creates a rate-limit error.
func NewRateLimitError(message string) *RateLimitError {
	return &RateLimitError{Message: message}
}

func (e *RateLimitError) Error() string {
	return "rate limit exceeded: " + e.Message
}

// IsRateLimitError reports whether err is a rate-limit error.
func IsRateLimitError(err error) bool {
	return isError[*RateLimitError](err)
}

// TokenExpiredError indicates the session token has expired
// (error_code -20651). Re-authenticate with login credentials.
type TokenExpiredError struct {
	Message string
}

// NewTokenExpiredError creates a token-expired error.
func NewTokenExpiredError(message string) *TokenExpiredError {
	return &TokenExpiredError{Message: message}
}

func (e *TokenExpiredError) Error() string {
	return "token expired: " + e.Message
}

// IsTokenExpiredError reports whether err is a token-expired error.
func IsTokenExpiredError(err error) bool {
	return isError[*TokenExpiredError](err)
}

// TokenNotSetError indicates an API call was attempted without
// providing an auth token.
type TokenNotSetError struct {
	Message string
}

// NewTokenNotSetError creates a token-not-set error.
func NewTokenNotSetError(message string) *TokenNotSetError {
	return &TokenNotSetError{Message: message}
}

func (e *TokenNotSetError) Error() string {
	return "token not set: " + e.Message
}

// IsTokenNotSetError reports whether err is a token-not-set error.
func IsTokenNotSetError(err error) bool {
	return isError[*TokenNotSetError](err)
}

// AuthenticationError indicates login failed (bad credentials).
type AuthenticationError struct {
	Message   string
	ErrorCode int
}

// NewAuthenticationError creates an authentication error.
func NewAuthenticationError(message string, errorCode int) *AuthenticationError {
	return &AuthenticationError{Message: message, ErrorCode: errorCode}
}

func (e *AuthenticationError) Error() string {
	return fmt.Sprintf("authentication error (code %d): %s", e.ErrorCode, e.Message)
}

// IsAuthenticationError reports whether err is an authentication error.
func IsAuthenticationError(err error) bool {
	return isError[*AuthenticationError](err)
}

// ParameterError indicates a bad request or parameter error
// (error_code -20104).
type ParameterError struct {
	Message string
}

// NewParameterError creates a parameter error.
func NewParameterError(message string) *ParameterError {
	return &ParameterError{Message: message}
}

func (e *ParameterError) Error() string {
	return "parameter error: " + e.Message
}

// IsParameterError reports whether err is a parameter error.
func IsParameterError(err error) bool {
	return isError[*ParameterError](err)
}

// UnsupportedOperationError indicates the device does not support
// the requested operation (device-level err_code -1).
type UnsupportedOperationError struct {
	Message string
}

// NewUnsupportedOperationError creates an unsupported-operation error.
func NewUnsupportedOperationError(message string) *UnsupportedOperationError {
	return &UnsupportedOperationError{Message: message}
}

func (e *UnsupportedOperationError) Error() string {
	return "unsupported operation: " + e.Message
}

// IsUnsupportedOperationError reports whether err is an unsupported-operation error.
func IsUnsupportedOperationError(err error) bool {
	return isError[*UnsupportedOperationError](err)
}

// NetworkError indicates a connectivity or transport failure.
type NetworkError struct {
	Message string
	Err     error
}

// NewNetworkError creates a network error.
func NewNetworkError(message string, err error) *NetworkError {
	return &NetworkError{Message: message, Err: err}
}

func (e *NetworkError) Error() string {
	// The underlying HTTP error may contain a request URL. Cloud tokens are
	// query parameters, so do not render the cause in the message. Callers can
	// still inspect it through Unwrap and errors.Is/errors.As.
	return "network error: " + e.Message
}

func (e *NetworkError) Unwrap() error { return e.Err }

// IsNetworkError reports whether err is a network error.
func IsNetworkError(err error) bool {
	return isError[*NetworkError](err)
}

// CloudAPIError represents a generic TP-Link Cloud API error that
// doesn't map to a more specific error type.
type CloudAPIError struct {
	ErrorCode int
	Message   string
}

// NewCloudAPIError creates a cloud API error.
func NewCloudAPIError(errorCode int, message string) *CloudAPIError {
	return &CloudAPIError{ErrorCode: errorCode, Message: message}
}

func (e *CloudAPIError) Error() string {
	return fmt.Sprintf("cloud API error (code %d): %s", e.ErrorCode, e.Message)
}

// IsCloudAPIError reports whether err is a cloud API error.
func IsCloudAPIError(err error) bool {
	return isError[*CloudAPIError](err)
}

// DeviceError represents a device-level error returned in passthrough
// responseData (non-zero err_code).
type DeviceError struct {
	ErrCode int
	ErrMsg  string
}

// NewDeviceError creates a device error.
func NewDeviceError(errCode int, errMsg string) *DeviceError {
	return &DeviceError{ErrCode: errCode, ErrMsg: errMsg}
}

func (e *DeviceError) Error() string {
	return fmt.Sprintf("device error (code %d): %s", e.ErrCode, e.ErrMsg)
}

// IsDeviceError reports whether err is a device error.
func IsDeviceError(err error) bool {
	return isError[*DeviceError](err)
}

func isError[T error](err error) bool {
	var target T

	return err != nil && errors.As(err, &target)
}
