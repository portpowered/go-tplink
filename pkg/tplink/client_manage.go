package tplink

import "context"

// SetAlias renames a device.
func (client *Client) SetAlias(ctx context.Context, request SetAliasRequest) error {
	cmd := map[string]any{
		NamespaceSystem: map[string]any{
			CmdSetDevAlias: map[string]any{"alias": request.Alias},
		},
	}
	data, err := client.doPassthrough(ctx, "SetAlias", request.Auth, request.DeviceID, cmd)
	if err != nil {
		return err
	}
	return checkPassthroughCommandError(data, NamespaceSystem, CmdSetDevAlias, "SetAlias")
}
