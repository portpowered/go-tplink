package tplink

import (
	"context"

	"github.com/portpowered/go-tplink/pkg/generatedwire"
)

// SetAlias renames a device.
func (client *Client) SetAlias(ctx context.Context, request SetAliasRequest) error {
	cmd := generatedwire.SystemSetDevAliasCommand{}
	cmd.System.SetDevAlias.Alias = request.Alias
	data, err := client.doPassthrough(ctx, "SetAlias", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}
	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetDevAlias, "SetAlias")
}
