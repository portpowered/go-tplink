package tplink

import (
	"context"
	"encoding/json"

	"github.com/portpowered/go-tplink/pkg/generatedwire"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// Login authenticates with the TP-Link Cloud API using email and password.
// Returns the session token and account information.
//
// This is the only method that does not require an auth token. The returned
// token should be supplied through AuthContext on each subsequent request.
func (client *Client) Login(ctx context.Context, request LoginRequest) (tplinkmodels.LoginResult, error) {
	cloudReq := generatedwire.LoginCloudRequest{
		Method: generatedwire.Login,
		Params: generatedwire.LoginParams{
			AppType:       generatedwire.TapoAndroid,
			CloudUserName: request.Email,
			CloudPassword: &request.Password,
			TerminalUUID:  generatedwire.GoTplinkClient,
		},
	}

	respBytes, err := client.doCloudRequest(ctx, "Login", cloudReq, nil)
	if err != nil {
		return tplinkmodels.LoginResult{}, err
	}

	err = checkCloudError(respBytes, "Login")
	if err != nil {
		return tplinkmodels.LoginResult{}, err
	}

	var loginResp generatedwire.LoginResponse

	err = json.Unmarshal(respBytes, &loginResp)
	if err != nil {
		return tplinkmodels.LoginResult{}, tplinkmodels.NewInvalidResponseError(
			"Login",
			"failed to parse login response",
			err,
		)
	}

	if loginResp.Result == nil {
		//nolint:exhaustruct // A missing provider result maps to the zero public result.
		return tplinkmodels.LoginResult{}, nil
	}

	return tplinkmodels.LoginResult{
		AccountID:   valueOrZero(loginResp.Result.AccountId),
		Token:       valueOrZero(loginResp.Result.Token),
		Email:       valueOrZero(loginResp.Result.Email),
		RegTime:     valueOrZero(loginResp.Result.RegTime),
		CountryCode: valueOrZero(loginResp.Result.CountryCode),
	}, nil
}
