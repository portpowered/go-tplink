package tplink

import (
	"context"

	"github.com/portpowered/go-tplink/pkg/dependencymodels"
)

// SetAlias renames a device.
func (client *Client) SetAlias(ctx context.Context, request SetAliasRequest) error {
	var cmd dependencymodels.SystemSetDevAliasCommand

	cmd.System.SetDevAlias.Alias = request.Alias

	data, err := client.doPassthrough(ctx, "SetAlias", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}

	return checkPassthroughCommandError(
		data,
		dependencymodels.NamespaceSystem,
		dependencymodels.CmdSetDevAlias,
		"SetAlias",
	)
}
