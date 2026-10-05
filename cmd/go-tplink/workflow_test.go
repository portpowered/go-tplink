package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
)

const workflowCommandCount = 5

//nolint:cyclop,funlen // This scenario verifies the saved-session workflow and failed re-login in sequence.
func TestCLIWorkflowLogsInListsAndControlsDiscoveredDevice(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	transport := &pairedTransport{next: 0, exchanges: []pairedExchange{
		{
			method:       http.MethodPost,
			url:          testBaseURL,
			body:         loginRequestBody,
			status:       http.StatusOK,
			responseBody: `{"error_code":0,"result":{"token":"` + testToken + `"}}`,
		},
		{
			method:       http.MethodPost,
			url:          testBaseURL + "?token=" + testToken,
			body:         deviceListRequestBody,
			status:       http.StatusOK,
			responseBody: deviceListResponseBody,
		},
		{
			method:       http.MethodPost,
			url:          testBaseURL + "?token=" + testToken,
			body:         plugOnRequestBody,
			status:       http.StatusOK,
			responseBody: plugMutationResponseBody,
		},
		{
			method:       http.MethodPost,
			url:          testBaseURL,
			body:         loginRequestBody,
			status:       http.StatusOK,
			responseBody: `{"error_code":-20601,"msg":"rejected ` + testEmail + ` ` + testPassword + `"}`,
		},
		{
			method:       http.MethodPost,
			url:          testBaseURL + "?token=" + testToken,
			body:         deviceListRequestBody,
			status:       http.StatusOK,
			responseBody: deviceListResponseBody,
		},
	}}
	deps, clients := workflowDependencies(t, tokenPath, transport)
	ctx := context.Background()

	loginOutput, err := runCommand(ctx, t, deps, []string{testAuthCommand, testLoginCommand})
	if err != nil {
		t.Fatalf("login: %v", err)
	}

	saved, err := (credentialStore{path: tokenPath}).load()
	if err != nil {
		t.Fatalf("load credentials saved by login: %v", err)
	}

	if saved.AccessToken != testToken {
		t.Fatal("login did not persist the paired session token")
	}

	if strings.Contains(loginOutput, testToken) || strings.Contains(loginOutput, testPassword) ||
		strings.Contains(loginOutput, testEmail) {
		t.Fatalf("login output exposed credentials: %q", loginOutput)
	}

	listOutput, err := runCommand(ctx, t, deps, []string{testDevicesCommand, testListCommand, testJSONFlag})
	if err != nil {
		t.Fatalf("list devices from the saved session: %v", err)
	}

	var devices []DeviceSummary

	err = json.Unmarshal([]byte(listOutput), &devices)
	if err != nil {
		t.Fatalf("decode device list: %v", err)
	}

	if len(devices) != 1 || devices[0].DeviceId != testPlugDevice || devices[0].Alias != "Desk Plug" {
		t.Fatalf("listed devices = %+v, want the paired plug", devices)
	}

	deviceID := devices[0].DeviceId

	operationOutput, err := runCommand(ctx, t, deps, []string{testPlugCommand, operationOn, deviceID})
	if err != nil {
		t.Fatalf("control discovered plug: %v", err)
	}

	if !strings.Contains(operationOutput, "Plug on request acknowledged") || strings.Contains(operationOutput, testToken) {
		t.Fatalf("plug operation output was unexpected: %q", operationOutput)
	}

	failedLoginOutput, err := runCommand(ctx, t, deps, []string{testAuthCommand, testLoginCommand})
	if err == nil {
		t.Fatal("re-login succeeded despite the paired authentication failure")
	}

	if strings.Contains(err.Error(), testEmail) || strings.Contains(err.Error(), testPassword) ||
		strings.Contains(err.Error(), testToken) || strings.Contains(failedLoginOutput, testPassword) {
		t.Fatalf("failed login exposed credentials: output=%q error=%v", failedLoginOutput, err)
	}

	preserved, err := (credentialStore{path: tokenPath}).load()
	if err != nil {
		t.Fatalf("failed re-login removed the previous credentials: %v", err)
	}

	if preserved.AccessToken != saved.AccessToken {
		t.Fatal("failed re-login replaced the previous session token")
	}

	afterFailureOutput, err := runCommand(ctx, t, deps, []string{testDevicesCommand, testListCommand, testJSONFlag})
	if err != nil {
		t.Fatalf("list devices after failed re-login: %v", err)
	}

	if !strings.Contains(afterFailureOutput, `"deviceId": "`+testPlugDevice+`"`) ||
		strings.Contains(afterFailureOutput, testToken) {
		t.Fatalf("post-failure device list was unexpected: %q", afterFailureOutput)
	}

	transport.assertConsumed(t)

	if len(*clients) != workflowCommandCount {
		t.Fatalf("created %d SDK clients, want %d", len(*clients), workflowCommandCount)
	}

	for index, client := range *clients {
		if client.closeCount != 1 {
			t.Errorf("SDK client %d close count = %d, want exactly one", index, client.closeCount)
		}
	}
}

func workflowDependencies(
	t *testing.T,
	tokenPath string,
	transport *pairedTransport,
) (dependencies, *[]*closeTrackingClient) {
	t.Helper()

	clients := make([]*closeTrackingClient, 0, workflowCommandCount)
	deps := dependencies{
		tokenPath: tokenPath,
		newClient: func() (tplink.ClientInterface, error) {
			client, err := tplink.NewClient(
				tplink.WithBaseURL(testBaseURL),
				tplink.WithHTTPClient(transport),
			)
			if err != nil {
				return nil, fmt.Errorf("create workflow SDK client: %w", err)
			}

			tracked := &closeTrackingClient{ClientInterface: client, closeCount: 0}
			clients = append(clients, tracked)

			return tracked, nil
		},
		getenv: func(name string) string {
			switch name {
			case testEmailEnvVar:
				return testEmail
			case testPasswordEnvVar:
				return testPassword
			default:
				return ""
			}
		},
		isTerminal: isTerminalReader,
	}

	return deps, &clients
}
