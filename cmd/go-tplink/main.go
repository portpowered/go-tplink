package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

type dependencies struct {
	tokenPath  string
	newClient  func() (tplink.ClientInterface, error)
	getenv     func(string) string
	isTerminal func(io.Reader) bool
}

const (
	operationState      = "state"
	operationOn         = "on"
	operationOff        = "off"
	operationReboot     = "reboot"
	operationBrightness = "brightness"
	operationColorTemp  = "color-temp"
	operationColor      = "color"
)

var errIntegerRequired = errors.New("must be an integer")

type commandFailureError string

func (failure commandFailureError) Error() string { return string(failure) }

func main() {
	err := execute()
	if err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)

		os.Exit(1)
	}
}

func execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	deps, err := defaultDependencies()
	if err != nil {
		return err
	}

	return runWithDependencies(ctx, os.Args[1:], os.Stdin, os.Stdout, deps)
}

func defaultDependencies() (dependencies, error) {
	configDirectory, err := os.UserConfigDir()
	if err != nil {
		return dependencies{}, fmt.Errorf("find user configuration directory: %w", err)
	}

	return dependencies{
		tokenPath: filepath.Join(configDirectory, "go-tplink", "credentials.json"),
		newClient: func() (tplink.ClientInterface, error) {
			return tplink.NewClient()
		},
		getenv:     os.Getenv,
		isTerminal: isTerminalReader,
	}, nil
}

func runWithDependencies(
	ctx context.Context,
	args []string,
	inputReader io.Reader,
	out io.Writer,
	deps dependencies,
) error {
	if len(args) == 0 {
		return writeUsage(out)
	}

	if deps.tokenPath == "" {
		return commandError("token file path is required")
	}

	deps = completeDependencies(deps)

	commandArgs, tokenPath, showHelp, err := parseRootFlags(args, deps.tokenPath)
	if err != nil {
		return err
	}

	if showHelp {
		return writeUsage(out)
	}

	if len(commandArgs) == 0 || commandArgs[0] == "help" {
		return writeUsage(out)
	}

	return dispatchCommand(ctx, credentialStore{path: tokenPath}, deps, commandArgs, inputReader, out)
}

func completeDependencies(deps dependencies) dependencies {
	if deps.getenv == nil {
		deps.getenv = os.Getenv
	}

	if deps.isTerminal == nil {
		deps.isTerminal = isTerminalReader
	}

	if deps.newClient == nil {
		deps.newClient = func() (tplink.ClientInterface, error) {
			return tplink.NewClient()
		}
	}

	return deps
}

func parseRootFlags(args []string, defaultTokenPath string) ([]string, string, bool, error) {
	rootFlags := flag.NewFlagSet("go-tplink", flag.ContinueOnError)
	rootFlags.SetOutput(io.Discard)
	tokenPath := rootFlags.String("token-file", defaultTokenPath, "credential token file")
	help := rootFlags.Bool("help", false, "show help")

	err := rootFlags.Parse(args)
	if errors.Is(err, flag.ErrHelp) || *help {
		return nil, "", true, nil
	}

	if err != nil {
		return nil, "", false, commandError("invalid global flags; run go-tplink --help")
	}

	return rootFlags.Args(), *tokenPath, false, nil
}

func dispatchCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	commandArgs []string,
	inputReader io.Reader,
	out io.Writer,
) error {
	switch commandArgs[0] {
	case "auth":
		return authCommand(ctx, store, deps, commandArgs[1:], inputReader, out)
	case "devices":
		return devicesCommand(ctx, store, deps, commandArgs[1:], out)
	case "plug":
		return plugCommand(ctx, store, deps, commandArgs[1:], out)
	case "bulb":
		return bulbCommand(ctx, store, deps, commandArgs[1:], out)
	case "device":
		return deviceCommand(ctx, store, deps, commandArgs[1:], out)
	default:
		return commandError("unknown command; run go-tplink --help")
	}
}

func authCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	inputReader io.Reader,
	out io.Writer,
) error {
	if len(args) == 0 {
		return commandError("usage: go-tplink auth login|status|export|logout")
	}

	switch args[0] {
	case "login":
		return loginCommand(ctx, store, deps, args[1:], inputReader, out)
	case "status":
		return authStatus(store, args[1:], out)
	case "export":
		return authExport(store, args[1:], out)
	case "logout":
		if len(args) != 1 {
			return commandError("usage: go-tplink auth logout")
		}

		err := store.remove()
		if err != nil {
			return fmt.Errorf("remove saved credentials: %w", err)
		}

		_, _ = fmt.Fprintln(out, "Saved credentials removed")

		return nil
	default:
		return commandError("unknown auth command; run go-tplink --help")
	}
}

func loginCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	inputReader io.Reader,
	out io.Writer,
) error {
	credentials, help, err := parseLoginInput(args, deps, inputReader, out)
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	return performLogin(ctx, store, deps, credentials, out)
}

func parseLoginInput(
	args []string,
	deps dependencies,
	inputReader io.Reader,
	out io.Writer,
) (LoginCredentials, bool, error) {
	var (
		stdinInput      bool
		credentialsFile string
	)

	remaining, help, err := parseCommandFlags("auth login", args, func(flags *flag.FlagSet) {
		flags.BoolVar(&stdinInput, "stdin", false, "read an email/password JSON object from stdin")
		flags.StringVar(&credentialsFile, "credentials-file", "", "read email and password from a protected JSON file")
	})
	if err != nil {
		return LoginCredentials{}, false, err
	}

	if help {
		return LoginCredentials{Email: "", Password: ""}, true, nil
	}

	if len(remaining) != 0 || (stdinInput && credentialsFile != "") {
		return LoginCredentials{}, false, commandError("usage: go-tplink auth login [--stdin | --credentials-file path]")
	}

	credentials, err := readLoginCredentials(stdinInput, credentialsFile, deps, inputReader, out)
	if err != nil {
		return LoginCredentials{}, false, err
	}

	if credentials.Email == "" || credentials.Password == "" {
		return LoginCredentials{}, false, commandError("email and password are required")
	}

	return credentials, false, nil
}

func performLogin(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	credentials LoginCredentials,
	out io.Writer,
) error {
	var resultToken string

	err := withClient(deps.newClient, func(client tplink.ClientInterface) error {
		result, loginErr := client.Login(ctx, tplink.LoginRequest{
			Email:    credentials.Email,
			Password: credentials.Password,
		})
		if loginErr != nil {
			return safeOperationError("login", loginErr, credentials.Email, credentials.Password)
		}

		if result.Token == "" {
			return commandError("login response did not include a session token")
		}

		resultToken = result.Token

		return nil
	})
	if err != nil {
		return err
	}

	err = store.save(StoredCredentials{AccessToken: resultToken})
	if err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	_, _ = fmt.Fprintln(out, "Login complete. The session token was saved with user-only file permissions.")

	return nil
}

func authStatus(store credentialStore, args []string, out io.Writer) error {
	var jsonOutput bool

	remaining, help, err := parseCommandFlags("auth status", args, func(flags *flag.FlagSet) {
		flags.BoolVar(&jsonOutput, "json", false, "write JSON")
	})
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	if len(remaining) != 0 {
		return commandError("usage: go-tplink auth status [--json]")
	}

	credentials, err := store.load()
	if err != nil {
		return err
	}

	if jsonOutput {
		return writeJSON(out, AuthStatusOutput{Authenticated: credentials.AccessToken != ""})
	}

	_, _ = fmt.Fprintln(out, "Authenticated; a saved session token is available")

	return nil
}

func authExport(store credentialStore, args []string, out io.Writer) error {
	var outputPath string

	remaining, help, err := parseCommandFlags("auth export", args, func(flags *flag.FlagSet) {
		flags.StringVar(&outputPath, "output", "", "write the access token as protected JSON")
	})
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	if len(remaining) != 0 || outputPath == "" {
		return commandError("usage: go-tplink auth export --output protected-file")
	}

	credentials, err := store.load()
	if err != nil {
		return err
	}

	err = writeProtectedExport(outputPath, StoredCredentials{AccessToken: credentials.AccessToken})
	if err != nil {
		return fmt.Errorf("export credentials: %w", err)
	}

	_, _ = fmt.Fprintf(out, "Credential export written to %s with user-only file permissions\n", outputPath)

	return nil
}

func devicesCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) == 0 || args[0] != "list" {
		return commandError("usage: go-tplink devices list [--json]")
	}

	var jsonOutput bool

	remaining, help, err := parseCommandFlags("devices list", args[1:], func(flags *flag.FlagSet) {
		flags.BoolVar(&jsonOutput, "json", false, "write JSON")
	})
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	if len(remaining) != 0 {
		return commandError("usage: go-tplink devices list [--json]")
	}

	return withAuthenticatedClient(
		store,
		deps,
		"list devices",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			result, requestErr := client.GetDevices(ctx, tplink.GetDevicesRequest{Auth: auth})
			if requestErr != nil {
				return safeOperationError("list devices", requestErr, auth.AccessToken)
			}

			summaries := make([]DeviceSummary, 0, len(result.Devices))
			for _, device := range result.Devices {
				summaries = append(summaries, DeviceSummary{
					DeviceId:   device.DeviceID,
					Alias:      device.Alias,
					Model:      device.DeviceModel,
					DeviceType: device.DeviceType,
					Status:     device.Status,
				})
			}

			return writeDeviceSummaries(out, summaries, jsonOutput)
		},
	)
}

func writeDeviceSummaries(out io.Writer, summaries []DeviceSummary, jsonOutput bool) error {
	if jsonOutput {
		return writeJSON(out, summaries)
	}

	for _, device := range summaries {
		_, _ = fmt.Fprintf(
			out,
			"%s\t%s\t%s\t%s\t%d\n",
			device.DeviceId,
			device.Alias,
			device.Model,
			device.DeviceType,
			device.Status,
		)
	}

	return nil
}

func plugCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) == 0 {
		return commandError("usage: go-tplink plug state|on|off|reboot")
	}

	switch args[0] {
	case operationState:
		return plugStateCommand(ctx, store, deps, args[1:], out)
	case operationOn, operationOff, operationReboot:
		if len(args) != 2 || args[1] == "" {
			return commandError("usage: go-tplink plug on|off|reboot DEVICE_ID")
		}

		return plugMutationCommand(ctx, store, deps, args[0], args[1], out)
	default:
		return commandError("unknown plug operation; run go-tplink --help")
	}
}

func plugStateCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	var jsonOutput bool

	remaining, help, err := parseCommandFlags("plug state", args, func(flags *flag.FlagSet) {
		flags.BoolVar(&jsonOutput, "json", false, "write JSON")
	})
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	if len(remaining) != 1 || remaining[0] == "" {
		return commandError("usage: go-tplink plug state [--json] DEVICE_ID")
	}

	deviceID := remaining[0]

	return withAuthenticatedClient(
		store,
		deps,
		"read plug state",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			state, requestErr := client.GetPowerState(ctx, tplink.GetPowerStateRequest{Auth: auth, DeviceID: deviceID})
			if requestErr != nil {
				return safeOperationError("read plug state", requestErr, auth.AccessToken)
			}

			if jsonOutput {
				return writeJSON(out, PowerStateOutput{IsOn: state.IsOn, OnTime: state.OnTime})
			}

			return printPowerState(out, state.IsOn, state.OnTime)
		},
	)
}

func plugMutationCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	operation string,
	deviceID string,
	out io.Writer,
) error {
	return withAuthenticatedClient(
		store,
		deps,
		"control plug",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			var (
				requestErr error
				label      string
			)

			switch operation {
			case operationOn:
				requestErr = client.TurnOn(ctx, tplink.TurnOnRequest{Auth: auth, DeviceID: deviceID})
				label = "Plug on request acknowledged"
			case operationOff:
				requestErr = client.TurnOff(ctx, tplink.TurnOffRequest{Auth: auth, DeviceID: deviceID})
				label = "Plug off request acknowledged"
			case operationReboot:
				requestErr = client.Reboot(ctx, tplink.RebootRequest{Auth: auth, DeviceID: deviceID})
				label = "Plug reboot request acknowledged"
			default:
				return commandError("unknown plug operation; run go-tplink --help")
			}

			if requestErr != nil {
				return safeOperationError("control plug", requestErr, auth.AccessToken)
			}

			_, _ = fmt.Fprintln(out, label)

			return nil
		},
	)
}

func bulbCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) == 0 {
		return commandError("usage: go-tplink bulb state|brightness|color-temp|color")
	}

	switch args[0] {
	case operationState:
		return bulbStateCommand(ctx, store, deps, args[1:], out)
	case operationBrightness:
		return bulbBrightnessCommand(ctx, store, deps, args[1:], out)
	case operationColorTemp:
		return bulbColorTempCommand(ctx, store, deps, args[1:], out)
	case operationColor:
		return bulbColorCommand(ctx, store, deps, args[1:], out)
	default:
		return commandError("unknown bulb operation; run go-tplink --help")
	}
}

func bulbStateCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	var jsonOutput bool

	remaining, help, err := parseCommandFlags("bulb state", args, func(flags *flag.FlagSet) {
		flags.BoolVar(&jsonOutput, "json", false, "write JSON")
	})
	if err != nil {
		return err
	}

	if help {
		return writeUsage(out)
	}

	if len(remaining) != 1 || remaining[0] == "" {
		return commandError("usage: go-tplink bulb state [--json] DEVICE_ID")
	}

	deviceID := remaining[0]

	return withAuthenticatedClient(
		store,
		deps,
		"read bulb state",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			state, requestErr := client.GetLightState(ctx, tplink.GetLightStateRequest{Auth: auth, DeviceID: deviceID})
			if requestErr != nil {
				return safeOperationError("read bulb state", requestErr, auth.AccessToken)
			}

			if jsonOutput {
				return writeJSON(out, LightStateOutput{
					IsOn:       state.OnOff != 0,
					Brightness: state.Brightness,
					Hue:        state.Hue,
					Saturation: state.Saturation,
					ColorTemp:  state.ColorTemp,
					Mode:       state.Mode,
				})
			}

			return printLightState(out, state)
		},
	)
}

func bulbBrightnessCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) != 2 || args[0] == "" {
		return commandError("usage: go-tplink bulb brightness DEVICE_ID 0-100")
	}

	brightness, err := parseInteger(args[1], operationBrightness)
	if err != nil {
		return err
	}

	if brightness < 0 || brightness > 100 {
		return commandError("brightness must be between 0 and 100")
	}

	deviceID := args[0]

	return withAuthenticatedClient(
		store,
		deps,
		"set bulb brightness",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			err := client.SetBrightness(ctx, tplink.SetBrightnessRequest{
				Auth:       auth,
				DeviceID:   deviceID,
				Brightness: brightness,
			})
			if err != nil {
				return safeOperationError("set bulb brightness", err, auth.AccessToken)
			}

			_, _ = fmt.Fprintln(out, "Bulb brightness request acknowledged")

			return nil
		},
	)
}

func bulbColorTempCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) != 2 || args[0] == "" {
		return commandError("usage: go-tplink bulb color-temp DEVICE_ID KELVIN")
	}

	colorTemp, err := parseInteger(args[1], "color temperature")
	if err != nil {
		return err
	}

	if colorTemp <= 0 {
		return commandError("color temperature must be greater than zero Kelvin")
	}

	deviceID := args[0]

	return withAuthenticatedClient(
		store,
		deps,
		"set bulb color temperature",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			err := client.SetColorTemp(ctx, tplink.SetColorTempRequest{
				Auth:      auth,
				DeviceID:  deviceID,
				ColorTemp: colorTemp,
			})
			if err != nil {
				return safeOperationError("set bulb color temperature", err, auth.AccessToken)
			}

			_, _ = fmt.Fprintln(out, "Bulb color temperature request acknowledged")

			return nil
		},
	)
}

func bulbColorCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) != 3 || args[0] == "" {
		return commandError("usage: go-tplink bulb color DEVICE_ID HUE SATURATION")
	}

	hue, err := parseInteger(args[1], "hue")
	if err != nil {
		return err
	}

	saturation, err := parseInteger(args[2], "saturation")
	if err != nil {
		return err
	}

	if hue < 0 || hue > 360 || saturation < 0 || saturation > 100 {
		return commandError("hue must be 0-360 and saturation 0-100")
	}

	deviceID := args[0]

	return withAuthenticatedClient(
		store,
		deps,
		"set bulb color",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			err := client.SetColor(ctx, tplink.SetColorRequest{
				Auth:       auth,
				DeviceID:   deviceID,
				Hue:        hue,
				Saturation: saturation,
			})
			if err != nil {
				return safeOperationError("set bulb color", err, auth.AccessToken)
			}

			_, _ = fmt.Fprintln(out, "Bulb color request acknowledged")

			return nil
		},
	)
}

func deviceCommand(
	ctx context.Context,
	store credentialStore,
	deps dependencies,
	args []string,
	out io.Writer,
) error {
	if len(args) < 1 || args[0] != "alias" || len(args) != 3 || args[1] == "" || strings.TrimSpace(args[2]) == "" {
		return commandError("usage: go-tplink device alias DEVICE_ID NEW_ALIAS")
	}

	deviceID := args[1]
	alias := args[2]

	return withAuthenticatedClient(
		store,
		deps,
		"rename device",
		func(client tplink.ClientInterface, auth tplink.AuthContext) error {
			err := client.SetAlias(ctx, tplink.SetAliasRequest{Auth: auth, DeviceID: deviceID, Alias: alias})
			if err != nil {
				return safeOperationError("rename device", err, auth.AccessToken)
			}

			_, _ = fmt.Fprintln(out, "Device alias request acknowledged")

			return nil
		},
	)
}

func withAuthenticatedClient(
	store credentialStore,
	deps dependencies,
	operation string,
	action func(tplink.ClientInterface, tplink.AuthContext) error,
) error {
	credentials, err := store.load()
	if err != nil {
		return fmt.Errorf("%s: %w", operation, err)
	}

	auth := tplink.AuthContext{AccessToken: credentials.AccessToken}

	return withClient(deps.newClient, func(client tplink.ClientInterface) error {
		err := action(client, auth)
		if err != nil {
			return err
		}

		return nil
	})
}

func withClient(
	factory func() (tplink.ClientInterface, error),
	action func(tplink.ClientInterface) error,
) (operationErr error) {
	client, err := factory()
	if err != nil {
		return fmt.Errorf("create TP-Link client: %w", err)
	}

	if client == nil {
		return commandError("client factory returned no client")
	}

	defer func() {
		closeErr := client.Close()
		if closeErr != nil && operationErr == nil {
			operationErr = fmt.Errorf("close TP-Link client: %w", closeErr)
		}
	}()

	return action(client)
}

func parseCommandFlags(name string, args []string, configure func(*flag.FlagSet)) ([]string, bool, error) {
	flags := flag.NewFlagSet(name, flag.ContinueOnError)
	flags.SetOutput(io.Discard)

	var help bool

	flags.BoolVar(&help, "help", false, "show help")
	configure(flags)

	err := flags.Parse(args)
	if errors.Is(err, flag.ErrHelp) {
		return nil, true, nil
	}

	if err != nil {
		return nil, false, commandError("invalid command flags; run go-tplink --help")
	}

	if help {
		return nil, true, nil
	}

	return flags.Args(), false, nil
}

func parseInteger(value, name string) (int, error) {
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", name, errIntegerRequired)
	}

	return parsed, nil
}

func printPowerState(out io.Writer, isOn bool, onTime int) error {
	state := "off"
	if isOn {
		state = "on"
	}

	_, err := fmt.Fprintf(out, "Power: %s\nOn time: %d seconds\n", state, onTime)
	if err != nil {
		return fmt.Errorf("write power state: %w", err)
	}

	return nil
}

func printLightState(out io.Writer, state tplinkmodels.LightState) error {
	_, err := fmt.Fprintf(
		out,
		"On: %t\nBrightness: %d\nHue: %d\nSaturation: %d\nColor temperature: %d K\nMode: %s\n",
		state.OnOff != 0,
		state.Brightness,
		state.Hue,
		state.Saturation,
		state.ColorTemp,
		state.Mode,
	)
	if err != nil {
		return fmt.Errorf("write light state: %w", err)
	}

	return nil
}

func writeJSON(out io.Writer, value any) error {
	encoder := json.NewEncoder(out)
	encoder.SetIndent("", "  ")

	err := encoder.Encode(value)
	if err != nil {
		return fmt.Errorf("write JSON output: %w", err)
	}

	return nil
}

func writeUsage(out io.Writer) error {
	const usage = `go-tplink — use the public go-tplink SDK from a terminal

Usage:
  go-tplink [--token-file path] auth login [--stdin | --credentials-file path]
  go-tplink auth status [--json]
  go-tplink auth export --output protected-file
  go-tplink auth logout
  go-tplink devices list [--json]
  go-tplink plug state [--json] DEVICE_ID
  go-tplink plug on|off|reboot DEVICE_ID
  go-tplink bulb state [--json] DEVICE_ID
  go-tplink bulb brightness DEVICE_ID 0-100
  go-tplink bulb color-temp DEVICE_ID KELVIN
  go-tplink bulb color DEVICE_ID HUE SATURATION
  go-tplink device alias DEVICE_ID NEW_ALIAS

Credentials:
  Set TPLINK_EMAIL and TPLINK_PASSWORD, use --stdin with a JSON object, or
  pass --credentials-file. Passwords and tokens are never command arguments.
  Auth login prompts with password echo disabled when attached to a terminal.
  Tokens are stored in a user-only credentials file and are never printed.
  auth export writes the token to a new user-only JSON file only when requested.

Every device change is an explicit command. Use --json with supported read
commands for machine-readable output. Press Ctrl-C to cancel an active request.`

	_, err := fmt.Fprintln(out, usage)
	if err != nil {
		return fmt.Errorf("write usage: %w", err)
	}

	return nil
}

func commandError(message string) error { return commandFailureError(message) }
