package tplinkmodels_test

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const (
	errorFixtureMessage       = "synthetic failure"
	errorFixtureOperation     = "synthetic operation"
	errorFixtureProviderCode  = -999
	errorFixtureResponseLimit = 1024
)

type publicErrorCase struct {
	name      string
	value     error
	recognize func(error) bool
	message   string
}

func wrappingErrorCases(cause error) []publicErrorCase {
	suffix := ""
	if cause != nil {
		suffix = ": " + cause.Error()
	}

	return []publicErrorCase{
		{"configuration", tplinkmodels.NewConfigurationError(errorFixtureMessage, cause), tplinkmodels.IsConfigurationError,
			"configuration error: " + errorFixtureMessage + suffix},
		{"request", tplinkmodels.NewInvalidRequestError(errorFixtureMessage, cause), tplinkmodels.IsInvalidRequestError,
			"invalid request: " + errorFixtureMessage + suffix},
		{"response",
			tplinkmodels.NewInvalidResponseError(errorFixtureOperation, errorFixtureMessage, cause),
			tplinkmodels.IsInvalidResponseError,
			errorFixtureOperation + ": invalid provider response: " + errorFixtureMessage + suffix},
		{"network", tplinkmodels.NewNetworkError(errorFixtureMessage, cause), tplinkmodels.IsNetworkError,
			"network error: " + errorFixtureMessage},
	}
}

func providerErrorCases() []publicErrorCase {
	return []publicErrorCase{
		{"http status", &tplinkmodels.HTTPStatusError{Operation: errorFixtureOperation, StatusCode: http.StatusNotFound},
			tplinkmodels.IsHTTPStatusError, errorFixtureOperation + ": provider returned HTTP status 404"},
		{"response size",
			&tplinkmodels.ResponseTooLargeError{Operation: errorFixtureOperation, Limit: errorFixtureResponseLimit},
			tplinkmodels.IsResponseTooLargeError, errorFixtureOperation + ": provider response exceeds 1024 bytes"},
		{"closed", &tplinkmodels.ClientClosedError{}, tplinkmodels.IsClientClosedError, "client is closed"},
		{"rate limit", tplinkmodels.NewRateLimitError(errorFixtureMessage), tplinkmodels.IsRateLimitError,
			"rate limit exceeded: " + errorFixtureMessage},
		{"expired", tplinkmodels.NewTokenExpiredError(errorFixtureMessage), tplinkmodels.IsTokenExpiredError,
			"token expired: " + errorFixtureMessage},
		{"missing token", tplinkmodels.NewTokenNotSetError(errorFixtureMessage), tplinkmodels.IsTokenNotSetError,
			"token not set: " + errorFixtureMessage},
		{"authentication",
			tplinkmodels.NewAuthenticationError(errorFixtureMessage, errorFixtureProviderCode),
			tplinkmodels.IsAuthenticationError,
			"authentication error (code -999): " + errorFixtureMessage},
		{"parameter", tplinkmodels.NewParameterError(errorFixtureMessage), tplinkmodels.IsParameterError,
			"parameter error: " + errorFixtureMessage},
		{"unsupported",
			tplinkmodels.NewUnsupportedOperationError(errorFixtureMessage),
			tplinkmodels.IsUnsupportedOperationError,
			"unsupported operation: " + errorFixtureMessage},
		{"cloud", tplinkmodels.NewCloudAPIError(errorFixtureProviderCode, errorFixtureMessage), tplinkmodels.IsCloudAPIError,
			"cloud API error (code -999): " + errorFixtureMessage},
		{"device", tplinkmodels.NewDeviceError(errorFixtureProviderCode, errorFixtureMessage), tplinkmodels.IsDeviceError,
			"device error (code -999): " + errorFixtureMessage},
	}
}

func assertPublicErrorClassification(t *testing.T, candidate publicErrorCase) {
	t.Helper()

	if candidate.value.Error() != candidate.message {
		t.Fatalf("Error() = %q, want %q", candidate.value.Error(), candidate.message)
	}

	wrapped := fmt.Errorf("caller context: %w", candidate.value)
	if !candidate.recognize(candidate.value) || !candidate.recognize(wrapped) {
		t.Fatal("public classifier did not recognize its direct and wrapped error")
	}

	if !errors.Is(wrapped, candidate.value) {
		t.Fatal("caller wrapping lost the original public error identity")
	}

	if candidate.recognize(nil) || candidate.recognize(io.EOF) {
		t.Fatal("public classifier accepted nil or an unrelated error")
	}
}

func TestPublicErrorWrappersRetainCause(t *testing.T) {
	t.Parallel()

	for _, cause := range []error{nil, io.ErrUnexpectedEOF} {
		for _, candidate := range wrappingErrorCases(cause) {
			t.Run(candidate.name+fmt.Sprint(cause), func(t *testing.T) {
				t.Parallel()

				assertPublicErrorClassification(t, candidate)

				if !errors.Is(errors.Unwrap(candidate.value), cause) {
					t.Fatal("Unwrap() lost or invented the supplied cause")
				}

				if cause != nil && !errors.Is(candidate.value, cause) {
					t.Fatal("errors.Is() did not reach the supplied cause")
				}
			})
		}
	}
}

func TestProviderErrorsRemainRecognizableThroughCallerWrapping(t *testing.T) {
	t.Parallel()

	for _, candidate := range providerErrorCases() {
		t.Run(candidate.name, func(t *testing.T) {
			t.Parallel()

			assertPublicErrorClassification(t, candidate)
		})
	}
}

func TestOptionalErrorContextAndEmptyResponseDetails(t *testing.T) {
	t.Parallel()

	cases := []publicErrorCase{
		{"status without operation", &tplinkmodels.HTTPStatusError{Operation: "", StatusCode: http.StatusNotFound},
			tplinkmodels.IsHTTPStatusError, "provider returned HTTP status 404"},
		{"size without operation", &tplinkmodels.ResponseTooLargeError{Operation: "", Limit: errorFixtureResponseLimit},
			tplinkmodels.IsResponseTooLargeError, "provider response exceeds 1024 bytes"},
		{"empty response",
			tplinkmodels.NewInvalidResponseError("", "", nil),
			tplinkmodels.IsInvalidResponseError, "invalid provider response"},
		{"response cause only",
			tplinkmodels.NewInvalidResponseError("", "", io.ErrUnexpectedEOF),
			tplinkmodels.IsInvalidResponseError,
			"invalid provider response: unexpected EOF"},
	}
	for _, candidate := range cases {
		t.Run(candidate.name, func(t *testing.T) {
			t.Parallel()

			assertPublicErrorClassification(t, candidate)
		})
	}
}

func TestNetworkErrorKeepsURLCauseWithoutLoggingCredentials(t *testing.T) {
	t.Parallel()

	const secret = "synthetic-token-not-for-output"

	cause := &url.Error{Op: "Post", URL: "https://example.invalid/?token=" + secret, Err: io.ErrUnexpectedEOF}

	networkError := tplinkmodels.NewNetworkError(errorFixtureMessage, cause)
	if strings.Contains(networkError.Error(), secret) || strings.Contains(networkError.Error(), cause.URL) {
		t.Fatal("network error message exposed a request URL or token")
	}

	var typedCause *url.Error

	if !errors.As(networkError, &typedCause) || typedCause != cause || !errors.Is(networkError, io.ErrUnexpectedEOF) {
		t.Fatal("redacted error lost the inspectable transport cause")
	}
}

func TestClassifyDeviceRetainsUnknownTypes(t *testing.T) {
	t.Parallel()

	cases := []struct {
		deviceType string
		want       tplinkmodels.EndpointType
	}{
		{dependencymodels.DeviceTypeSmartPlug, tplinkmodels.EndpointTypePlug},
		{dependencymodels.DeviceTypeRangeExtenderPlug, tplinkmodels.EndpointTypePlug},
		{dependencymodels.DeviceTypeSmartBulb, tplinkmodels.EndpointTypeBulb},
		{"", tplinkmodels.EndpointTypeOther},
		{"synthetic-future-device-type", tplinkmodels.EndpointTypeOther},
	}
	for _, candidate := range cases {
		t.Run(candidate.deviceType, func(t *testing.T) {
			t.Parallel()

			if got := tplinkmodels.ClassifyDevice(candidate.deviceType); got != candidate.want {
				t.Fatalf("ClassifyDevice(%q) = %q, want %q", candidate.deviceType, got, candidate.want)
			}
		})
	}
}
