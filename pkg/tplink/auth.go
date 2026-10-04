package tplink

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/portpowered/go-tplink/pkg/dependencies/cloud"
	"github.com/portpowered/go-tplink/pkg/dependencymodels"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

// Login authenticates with the TP-Link Cloud API using email and password.
// Returns the session token and account information.
//
// This is the only method that does not require an auth token. The returned
// token should be supplied through AuthContext on each subsequent request.
func (client *Client) Login(ctx context.Context, request LoginRequest) (tplinkmodels.LoginResult, error) {
	cloudReq := dependencymodels.LoginCloudRequest{
		Method: dependencymodels.MethodLogin,
		Params: dependencymodels.LoginParams{
			AppType:       dependencymodels.AppType,
			CloudUserName: request.Email,
			CloudPassword: &request.Password,
			TerminalUUID:  dependencymodels.TerminalUUID,
		},
	}

	respBytes, err := client.doCloudRequest(ctx, "Login", cloudReq, nil)
	if err != nil {
		return tplinkmodels.LoginResult{}, err
	}

	err = cloud.CheckError(respBytes, "Login")
	if err != nil {
		return tplinkmodels.LoginResult{}, fmt.Errorf("%w", err)
	}

	var loginResp dependencymodels.LoginResponse

	err = json.Unmarshal(respBytes, &loginResp)
	if err != nil {
		return tplinkmodels.LoginResult{}, tplinkmodels.NewInvalidResponseError(
			"Login",
			"failed to parse login response",
			err,
		)
	}

	if loginResp.Result == nil {
		var result tplinkmodels.LoginResult

		return result, nil
	}

	return tplinkmodels.LoginResult{
		AccountID:   valueOrZero(loginResp.Result.AccountId),
		Token:       valueOrZero(loginResp.Result.Token),
		Email:       valueOrZero(loginResp.Result.Email),
		RegTime:     valueOrZero(loginResp.Result.RegTime),
		CountryCode: valueOrZero(loginResp.Result.CountryCode),
	}, nil
}
