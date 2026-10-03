package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/portpowered/go-tplink/pkg/tplink"
)

const (
	testBaseURL  = "https://api.example.test"
	testToken    = "session-token-secret"
	testEmail    = "person@example.com"
	testPassword = "password-not-output"
)

const loginRequestBody = `{"method":"login","params":{"appType":"Tapo_Android",` +
	`"cloudPassword":"` + testPassword + `","cloudUserName":"` + testEmail + `",` +
	`"terminalUUID":"go-tplink-client"}}`

const stdinCredentialsJSON = `{"email":"` + testEmail + `","password":"stdin-secret"}`

const deviceListResponseBody = `{"error_code":0,"result":{"deviceList":[{"deviceType":"plug",` +
	`"deviceId":"plug-1","alias":"Desk Plug","deviceModel":"P100",` +
	`"deviceMac":"AA:BB:CC:DD:EE:FF","status":1}]}}`

const plugStateRequestBody = `{"method":"passthrough","params":{"deviceId":"plug-1",` +
	`"requestData":"{\"system\":{\"get_sysinfo\":\"\"}}"}}`

const plugStateResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"system\":{\"get_sysinfo\":{\"relay_state\":1,\"on_time\":12}}}"}}`

const bulbStateRequestBody = `{"method":"passthrough","params":{"deviceId":"bulb-1",` +
	`"requestData":"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"get_light_state\":\"\"}}"}}`

const bulbStateResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"smartlife.iot.smartbulb.lightingservice\":{\"get_light_state\":` +
	`{\"on_off\":1,\"brightness\":73,\"hue\":190,\"saturation\":60,\"color_temp\":4000}}}"}}`

const bulbBrightnessRequestBody = `{"method":"passthrough","params":{"deviceId":"bulb-1",` +
	`"requestData":"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"transition_light_state\":{\"brightness\":65}}}"}}`

const bulbBrightnessResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"transition_light_state\":{\"err_code\":0}}}"}}`

const deviceAliasRequestBody = `{"method":"passthrough","params":{"deviceId":"bulb-1",` +
	`"requestData":"{\"system\":{\"set_dev_alias\":{\"alias\":` +
	`\"Bedroom Light\"}}}"}}`

const deviceAliasResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"system\":{\"set_dev_alias\":{\"err_code\":0}}}"}}`

const plugOnRequestBody = `{"method":"passthrough","params":{"deviceId":"plug-1",` +
	`"requestData":"{\"system\":{\"set_relay_state\":{\"state\":1}}}"}}`

const plugOffRequestBody = `{"method":"passthrough","params":{"deviceId":"plug-1",` +
	`"requestData":"{\"system\":{\"set_relay_state\":{\"state\":0}}}"}}`

const plugRebootRequestBody = `{"method":"passthrough","params":{"deviceId":"plug-1",` +
	`"requestData":"{\"system\":{\"reboot\":{\"delay\":1}}}"}}`

const plugMutationResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"system\":{\"set_relay_state\":{\"err_code\":0}}}"}}`

const plugRebootResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"system\":{\"reboot\":{\"err_code\":0}}}"}}`

const bulbColorTempRequestBody = `{"method":"passthrough","params":{"deviceId":"bulb-1",` +
	`"requestData":"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"transition_light_state\":{\"color_temp\":4000}}}"}}`

const bulbColorRequestBody = `{"method":"passthrough","params":{"deviceId":"bulb-1",` +
	`"requestData":"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"transition_light_state\":{\"hue\":190,\"saturation\":60}}}"}}`

const bulbMutationResponseBody = `{"error_code":0,"result":{"responseData":` +
	`"{\"smartlife.iot.smartbulb.lightingservice\":` +
	`{\"transition_light_state\":{\"err_code\":0}}}"}}`

var (
	errUnpairedRequest       = errors.New("unexpected request without a paired response")
	errPairedRequestMismatch = errors.New("request did not match its paired expectation")
)

type pairedExchange struct {
	method       string
	url          string
	body         string
	status       int
	responseBody string
}

type pairedTransport struct {
	exchanges []pairedExchange
	next      int
}

func (transport *pairedTransport) Do(request *http.Request) (*http.Response, error) {
	err := request.Context().Err()
	if err != nil {
		return nil, fmt.Errorf("request context: %w", err)
	}

	if transport.next >= len(transport.exchanges) {
		return nil, errUnpairedRequest
	}

	expected := transport.exchanges[transport.next]

	requestBody, err := io.ReadAll(request.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}

	err = request.Body.Close()
	if err != nil {
		return nil, fmt.Errorf("close request body: %w", err)
	}

	if request.Method != expected.method || request.URL.String() != expected.url ||
		request.Header.Get("Content-Type") != "application/json" || string(requestBody) != expected.body {
		return nil, errPairedRequestMismatch
	}

	transport.next++

	status := expected.status
	if status == 0 {
		status = http.StatusOK
	}

	recorder := httptest.NewRecorder()
	recorder.Header().Set("Content-Type", "application/json")
	recorder.WriteHeader(status)
	_, _ = recorder.WriteString(expected.responseBody)
	response := recorder.Result()
	response.Request = request

	return response, nil
}

func (transport *pairedTransport) assertConsumed(t *testing.T) {
	t.Helper()

	if transport.next != len(transport.exchanges) {
		t.Fatalf("consumed %d of %d paired request/response expectations", transport.next, len(transport.exchanges))
	}
}

type closeTrackingClient struct {
	tplink.ClientInterface

	closeCount int
}

func (client *closeTrackingClient) Close() error {
	client.closeCount++

	err := client.ClientInterface.Close()
	if err != nil {
		return fmt.Errorf("close test client: %w", err)
	}

	return nil
}

func dependenciesForTransport(
	t *testing.T,
	tokenPath string,
	transport *pairedTransport,
) (dependencies, *closeTrackingClient) {
	t.Helper()

	client, err := tplink.NewClient(
		tplink.WithBaseURL(testBaseURL),
		tplink.WithHTTPClient(transport),
	)
	if err != nil {
		t.Fatalf("create test client: %v", err)
	}

	tracked := &closeTrackingClient{ClientInterface: client, closeCount: 0}
	deps := dependencies{
		tokenPath:  tokenPath,
		getenv:     func(string) string { return "" },
		isTerminal: isTerminalReader,
		newClient: func() (tplink.ClientInterface, error) {
			return tracked, nil
		},
	}

	return deps, tracked
}

func saveTestToken(t *testing.T, path string) {
	t.Helper()

	err := (credentialStore{path: path}).save(storedCredentials{AccessToken: testToken})
	if err != nil {
		t.Fatalf("save test token: %v", err)
	}
}

func runCommand(ctx context.Context, t *testing.T, deps dependencies, args []string) (string, error) {
	t.Helper()

	var output strings.Builder

	err := runWithDependencies(ctx, args, strings.NewReader(""), &output, deps)

	return output.String(), err
}

//nolint:cyclop // One paired login scenario checks wire matching, secret handling, token storage, and cleanup.
func TestLoginUsesPairedRequestAndStoresTokenWithoutPrintingSecrets(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	transport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
		method:       http.MethodPost,
		url:          testBaseURL,
		body:         loginRequestBody,
		status:       http.StatusOK,
		responseBody: `{"error_code":0,"result":{"token":"` + testToken + `"}}`,
	}}}
	deps, tracked := dependenciesForTransport(t, tokenPath, transport)
	deps.getenv = func(name string) string {
		switch name {
		case "TPLINK_EMAIL":
			return testEmail
		case "TPLINK_PASSWORD":
			return testPassword
		default:
			return ""
		}
	}

	output, err := runCommand(context.Background(), t, deps, []string{"auth", "login"})
	if err != nil {
		t.Fatalf("login failed: %v", err)
	}

	if strings.Contains(output, testToken) || strings.Contains(output, testPassword) ||
		strings.Contains(output, testEmail) {
		t.Fatalf("login output contains credential material: %q", output)
	}

	if strings.Contains(output, "Login complete") == false {
		t.Fatalf("login output did not confirm completion: %q", output)
	}

	credentials, err := (credentialStore{path: tokenPath}).load()
	if err != nil {
		t.Fatalf("load saved token: %v", err)
	}

	if credentials.AccessToken != testToken {
		t.Fatal("saved token did not match the paired login response")
	}

	transport.assertConsumed(t)

	if tracked == nil || tracked.closeCount != 1 {
		t.Fatalf("client close count = %v, want 1", tracked)
	}
}

func TestLoginAuthFailureRedactsSecretsAndClosesClient(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	transport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
		method:       http.MethodPost,
		url:          testBaseURL,
		body:         loginRequestBody,
		status:       http.StatusOK,
		responseBody: `{"error_code":-20601,"msg":"rejected ` + testEmail + ` ` + testPassword + `"}`,
	}}}
	deps, tracked := dependenciesForTransport(t, tokenPath, transport)
	deps.getenv = func(name string) string {
		if name == "TPLINK_EMAIL" {
			return testEmail
		}

		if name == "TPLINK_PASSWORD" {
			return testPassword
		}

		return ""
	}

	_, err := runCommand(context.Background(), t, deps, []string{"auth", "login"})
	if err == nil {
		t.Fatal("login succeeded despite a paired authentication failure")
	}

	if strings.Contains(err.Error(), testPassword) || strings.Contains(err.Error(), testEmail) {
		t.Fatalf("login error contains credential material: %v", err)
	}

	if !strings.Contains(err.Error(), "login") {
		t.Fatalf("login error lacks operation context: %v", err)
	}

	_, statErr := os.Stat(tokenPath)
	if !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("failed authentication created a token file: %v", statErr)
	}

	transport.assertConsumed(t)

	if tracked == nil || tracked.closeCount != 1 {
		t.Fatalf("client close count = %v, want 1", tracked)
	}
}

//nolint:cyclop // This paired scenario checks both discovery and plug state with independent client lifecycles.
func TestDevicesListAndPlugStateConsumePairedExchanges(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	saveTestToken(t, tokenPath)

	listTransport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
		method:       http.MethodPost,
		url:          testBaseURL + "?token=" + testToken,
		body:         `{"method":"getDeviceList"}`,
		status:       http.StatusOK,
		responseBody: deviceListResponseBody,
	}}}
	listDeps, listClient := dependenciesForTransport(t, tokenPath, listTransport)

	listOutput, err := runCommand(context.Background(), t, listDeps, []string{"devices", "list", "--json"})
	if err != nil {
		t.Fatalf("list devices: %v", err)
	}

	if !strings.Contains(listOutput, "plug-1") || strings.Contains(listOutput, "AA:BB:CC:DD:EE:FF") ||
		strings.Contains(listOutput, testToken) {
		t.Fatalf("device list output has unexpected fields or secrets: %q", listOutput)
	}

	listTransport.assertConsumed(t)

	if listClient == nil || listClient.closeCount != 1 {
		t.Fatal("device listing did not close its client exactly once")
	}

	stateTransport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
		method:       http.MethodPost,
		url:          testBaseURL + "?token=" + testToken,
		body:         plugStateRequestBody,
		status:       http.StatusOK,
		responseBody: plugStateResponseBody,
	}}}
	stateDeps, stateClient := dependenciesForTransport(t, tokenPath, stateTransport)

	stateOutput, err := runCommand(context.Background(), t, stateDeps, []string{"plug", "state", "--json", "plug-1"})
	if err != nil {
		t.Fatalf("read plug state: %v", err)
	}

	if !strings.Contains(stateOutput, `"isOn": true`) || !strings.Contains(stateOutput, `"onTime": 12`) ||
		strings.Contains(stateOutput, testToken) {
		t.Fatalf("plug state output did not contain expected values: %q", stateOutput)
	}

	stateTransport.assertConsumed(t)

	if stateClient == nil || stateClient.closeCount != 1 {
		t.Fatal("plug state did not close its client exactly once")
	}
}

//nolint:funlen // The table covers each supported bulb workflow with a paired wire response.
func TestBulbReadControlAndAliasUsePairedRequests(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	saveTestToken(t, tokenPath)

	tests := []struct {
		name       string
		args       []string
		request    string
		response   string
		wantOutput string
	}{
		{
			name:       "bulb state",
			args:       []string{"bulb", "state", "--json", "bulb-1"},
			request:    bulbStateRequestBody,
			response:   bulbStateResponseBody,
			wantOutput: `"brightness": 73`,
		},
		{
			name:       "bulb brightness",
			args:       []string{"bulb", "brightness", "bulb-1", "65"},
			request:    bulbBrightnessRequestBody,
			response:   bulbBrightnessResponseBody,
			wantOutput: "Bulb brightness request acknowledged",
		},
		{
			name:       "bulb color temperature",
			args:       []string{"bulb", "color-temp", "bulb-1", "4000"},
			request:    bulbColorTempRequestBody,
			response:   bulbMutationResponseBody,
			wantOutput: "Bulb color temperature request acknowledged",
		},
		{
			name:       "bulb hue and saturation",
			args:       []string{"bulb", "color", "bulb-1", "190", "60"},
			request:    bulbColorRequestBody,
			response:   bulbMutationResponseBody,
			wantOutput: "Bulb color request acknowledged",
		},
		{
			name:       "device alias",
			args:       []string{"device", "alias", "bulb-1", "Bedroom Light"},
			request:    deviceAliasRequestBody,
			response:   deviceAliasResponseBody,
			wantOutput: "Device alias request acknowledged",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			transport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
				method:       http.MethodPost,
				url:          testBaseURL + "?token=" + testToken,
				body:         test.request,
				status:       http.StatusOK,
				responseBody: test.response,
			}}}
			deps, tracked := dependenciesForTransport(t, tokenPath, transport)

			output, err := runCommand(context.Background(), t, deps, test.args)
			if err != nil {
				t.Fatalf("command failed: %v", err)
			}

			if !strings.Contains(output, test.wantOutput) || strings.Contains(output, testToken) {
				t.Fatalf("unexpected command output: %q", output)
			}

			transport.assertConsumed(t)

			if tracked == nil || tracked.closeCount != 1 {
				t.Fatal("command did not close its client exactly once")
			}
		})
	}
}

//nolint:funlen // The table verifies each explicitly requested plug mutation and its response.
func TestPlugMutationsUsePairedRequests(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	saveTestToken(t, tokenPath)

	tests := []struct {
		name       string
		operation  string
		request    string
		response   string
		wantOutput string
	}{
		{
			name:       "turn on",
			operation:  "on",
			request:    plugOnRequestBody,
			response:   plugMutationResponseBody,
			wantOutput: "Plug on request acknowledged",
		},
		{
			name:       "turn off",
			operation:  "off",
			request:    plugOffRequestBody,
			response:   plugMutationResponseBody,
			wantOutput: "Plug off request acknowledged",
		},
		{
			name:       "reboot",
			operation:  "reboot",
			request:    plugRebootRequestBody,
			response:   plugRebootResponseBody,
			wantOutput: "Plug reboot request acknowledged",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			transport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
				method:       http.MethodPost,
				url:          testBaseURL + "?token=" + testToken,
				body:         test.request,
				status:       http.StatusOK,
				responseBody: test.response,
			}}}
			deps, tracked := dependenciesForTransport(t, tokenPath, transport)

			output, err := runCommand(
				context.Background(),
				t,
				deps,
				[]string{"plug", test.operation, "plug-1"},
			)
			if err != nil {
				t.Fatalf("command failed: %v", err)
			}

			if !strings.Contains(output, test.wantOutput) || strings.Contains(output, testToken) {
				t.Fatalf("unexpected command output: %q", output)
			}

			transport.assertConsumed(t)

			if tracked.closeCount != 1 {
				t.Fatal("command did not close its client exactly once")
			}
		})
	}
}

func TestCanceledRequestClosesClientWithoutConsumingPair(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	saveTestToken(t, tokenPath)

	transport := &pairedTransport{next: 0, exchanges: []pairedExchange{{
		method:       http.MethodPost,
		url:          testBaseURL + "?token=" + testToken,
		body:         `{"method":"getDeviceList"}`,
		status:       http.StatusOK,
		responseBody: `{"error_code":0,"result":{"deviceList":[]}}`,
	}}}
	deps, tracked := dependenciesForTransport(t, tokenPath, transport)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runCommand(ctx, t, deps, []string{"devices", "list"})
	if err == nil || !strings.Contains(err.Error(), "canceled") || strings.Contains(err.Error(), testToken) {
		t.Fatalf("unexpected canceled request error: %v", err)
	}

	if transport.next != 0 {
		t.Fatalf("canceled request consumed %d paired responses", transport.next)
	}

	if tracked == nil || tracked.closeCount != 1 {
		t.Fatal("canceled request did not close its client exactly once")
	}
}

func TestCredentialExportIsExplicitProtectedAndDoesNotOverwrite(t *testing.T) {
	t.Parallel()

	tokenPath := filepath.Join(t.TempDir(), "credentials.json")
	saveTestToken(t, tokenPath)
	deps := dependencies{
		tokenPath:  tokenPath,
		newClient:  nil,
		getenv:     nil,
		isTerminal: nil,
	}
	exportPath := filepath.Join(t.TempDir(), "export.json")

	output, err := runCommand(context.Background(), t, deps, []string{"auth", "export", "--output", exportPath})
	if err != nil {
		t.Fatalf("export credentials: %v", err)
	}

	if strings.Contains(output, testToken) {
		t.Fatalf("export confirmation printed the token: %q", output)
	}

	// The export path is created by this test inside t.TempDir.
	//nolint:gosec // This is the explicit output path under the isolated test directory.
	data, err := os.ReadFile(exportPath)
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	if !strings.Contains(string(data), testToken) {
		t.Fatal("explicit export did not contain the requested token")
	}

	_, err = runCommand(
		context.Background(),
		t,
		deps,
		[]string{"auth", "export", "--output", exportPath},
	)
	if err == nil {
		t.Fatal("credential export overwrote an existing file")
	}

	info, err := os.Stat(exportPath)
	if err != nil {
		t.Fatalf("stat export: %v", err)
	}

	if runtime.GOOS != "windows" && info.Mode().Perm() != privateFileMode {
		t.Fatalf("export mode = %o, want %o", info.Mode().Perm(), privateFileMode)
	}
}

func TestLoginReadsCredentialsFromStdinAndRejectsTrailingJSON(t *testing.T) {
	t.Parallel()

	good, err := decodeLoginCredentials(strings.NewReader(stdinCredentialsJSON))
	if err != nil {
		t.Fatalf("decode stdin credentials: %v", err)
	}

	if good.Email != testEmail || good.Password != "stdin-secret" {
		t.Fatal("decoded credentials did not match the input")
	}

	trailingInput := strings.NewReader(stdinCredentialsJSON + ` {}`)

	_, err = decodeLoginCredentials(trailingInput)
	if err == nil {
		t.Fatal("accepted trailing stdin data")
	}

	extraCredential := `{"email":"` + testEmail + `","password":"stdin-secret","token":"extra"}`
	extraInput := strings.NewReader(extraCredential)

	_, err = decodeLoginCredentials(extraInput)
	if err == nil {
		t.Fatal("accepted unknown credential fields")
	}
}
