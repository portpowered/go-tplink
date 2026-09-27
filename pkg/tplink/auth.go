package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// Login authenticates with the TP-Link Cloud API using email and password.
// Returns the session token and account information.
//
// This is the only method that does not require an auth token. The returned
// token should be supplied through AuthContext on each subsequent request.
func (client *Client) Login(ctx context.Context, request LoginRequest) (tplinkmodels.LoginResult, error) {
	cloudReq := tplinkmodels.CloudRequest{
		Method: MethodLogin,
		Params: tplinkmodels.LoginParams{
			AppType:       AppType,
			CloudUserName: request.Email,
			CloudPassword: request.Password,
			TerminalUUID:  "go-tplink-client",
		},
	}

	respBytes, err := client.doCloudRequest(ctx, "Login", cloudReq, nil)
	if err != nil {
		return tplinkmodels.LoginResult{}, err
	}

	if err := checkCloudError(respBytes, "Login"); err != nil {
		return tplinkmodels.LoginResult{}, err
	}

	var loginResp tplinkmodels.LoginResponse
	if err := json.Unmarshal(respBytes, &loginResp); err != nil {
		return tplinkmodels.LoginResult{}, tplinkmodels.NewInvalidResponseError("Login", "failed to parse login response", err)
	}

	return loginResp.Result, nil
}
