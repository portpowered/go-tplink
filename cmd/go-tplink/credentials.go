// Package main implements the standalone go-tplink command-line client.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/term"
)

const (
	privateFileMode        = 0o600
	privateDirMode         = 0o700
	maxCredentialJSONBytes = 64 << 10
)

var (
	tokenQueryPattern          = regexp.MustCompile(`(?i)([?&]token=)[^&\s]+`)
	errRequestCanceled         = errors.New("request canceled")
	errRequestDeadlineExceeded = errors.New("request deadline exceeded")
)

type operationError struct {
	operation string
	message   string
}

func (err operationError) Error() string {
	return err.operation + ": " + err.message
}

type loginCredentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type storedCredentials struct {
	AccessToken string `json:"accessToken"`
}

type credentialStore struct {
	path string
}

func readLoginCredentials(
	stdinInput bool,
	credentialsFile string,
	deps dependencies,
	inputReader io.Reader,
	out io.Writer,
) (loginCredentials, error) {
	if stdinInput {
		return decodeLoginCredentials(inputReader)
	}

	if credentialsFile != "" {
		return readLoginCredentialsFile(credentialsFile)
	}

	email := deps.getenv("TPLINK_EMAIL")

	password := deps.getenv("TPLINK_PASSWORD")

	if email != "" || password != "" {
		if email == "" || password == "" {
			return loginCredentials{}, commandError("set both TPLINK_EMAIL and TPLINK_PASSWORD")
		}

		return loginCredentials{Email: email, Password: password}, nil
	}

	if deps.isTerminal(inputReader) {
		return promptLoginCredentials(inputReader, out)
	}

	return loginCredentials{}, commandError("provide TPLINK_EMAIL and TPLINK_PASSWORD, --stdin, or --credentials-file")
}

func decodeLoginCredentials(input io.Reader) (loginCredentials, error) {
	decoder := json.NewDecoder(io.LimitReader(input, maxCredentialJSONBytes))
	decoder.DisallowUnknownFields()

	var credentials loginCredentials

	err := decoder.Decode(&credentials)
	if err != nil {
		return loginCredentials{}, commandError("stdin credentials must be a JSON object with email and password")
	}

	var trailing any

	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		return loginCredentials{}, commandError("stdin credentials must contain one JSON object")
	}

	return credentials, nil
}

func readLoginCredentialsFile(path string) (loginCredentials, error) {
	// This explicit path is supplied by the caller as a local credentials file.
	//nolint:gosec // The CLI intentionally reads user-selected credential files.
	file, err := os.Open(path)
	if err != nil {
		return loginCredentials{}, fmt.Errorf("open credentials file: %w", err)
	}

	defer func() { _ = file.Close() }()

	return decodeLoginCredentials(file)
}

func promptLoginCredentials(inputReader io.Reader, out io.Writer) (loginCredentials, error) {
	file, isFile := inputReader.(*os.File)

	if !isFile || !term.IsTerminal(int(file.Fd())) {
		return loginCredentials{}, commandError("interactive credentials require a terminal")
	}

	reader := bufio.NewReader(inputReader)
	_, _ = fmt.Fprint(out, "Email: ")

	email, err := reader.ReadString('\n')

	if err != nil && !errors.Is(err, io.EOF) {
		return loginCredentials{}, fmt.Errorf("read email: %w", err)
	}

	_, _ = fmt.Fprint(out, "Password: ")

	passwordBytes, err := term.ReadPassword(int(file.Fd()))

	_, _ = fmt.Fprintln(out)

	if err != nil {
		return loginCredentials{}, fmt.Errorf("read password: %w", err)
	}

	return loginCredentials{Email: strings.TrimSpace(email), Password: string(passwordBytes)}, nil
}

func (store credentialStore) load() (storedCredentials, error) {
	var credentials storedCredentials

	file, err := os.Open(store.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return credentials, commandError("no saved credentials; run go-tplink auth login")
		}

		return credentials, fmt.Errorf("open saved credentials: %w", err)
	}

	defer func() { _ = file.Close() }()

	decoder := json.NewDecoder(io.LimitReader(file, maxCredentialJSONBytes))
	decoder.DisallowUnknownFields()

	err = decoder.Decode(&credentials)
	if err != nil || credentials.AccessToken == "" {
		return storedCredentials{}, commandError("saved credentials file is invalid; run go-tplink auth login")
	}

	var trailing any

	err = decoder.Decode(&trailing)
	if !errors.Is(err, io.EOF) {
		return storedCredentials{}, commandError("saved credentials file has trailing data")
	}

	return credentials, nil
}

func (store credentialStore) save(credentials storedCredentials) error {
	if credentials.AccessToken == "" {
		return commandError("cannot save an empty session token")
	}

	directory := filepath.Dir(store.path)

	err := os.MkdirAll(directory, privateDirMode)
	if err != nil {
		return fmt.Errorf("create credential directory: %w", err)
	}

	err = restrictCredentialPath(directory)
	if err != nil {
		return fmt.Errorf("protect credential directory: %w", err)
	}

	//nolint:gosec // Login deliberately persists the token in a private mode-0600 file.
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode saved credentials: %w", err)
	}

	file, err := os.CreateTemp(directory, ".credentials-*")
	if err != nil {
		return fmt.Errorf("create temporary credential file: %w", err)
	}

	temporaryPath := file.Name()

	defer func() { _ = os.Remove(temporaryPath) }()

	defer func() { _ = file.Close() }()

	err = writeTemporaryCredential(file, temporaryPath, data)
	if err != nil {
		return err
	}

	err = replaceCredentialFile(temporaryPath, store.path)
	if err != nil {
		return fmt.Errorf("save credentials: %w", err)
	}

	return nil
}

func writeTemporaryCredential(file *os.File, path string, data []byte) error {
	err := file.Chmod(privateFileMode)
	if err != nil {
		return fmt.Errorf("set credential file permissions: %w", err)
	}

	err = restrictCredentialPath(path)
	if err != nil {
		return fmt.Errorf("protect credential file: %w", err)
	}

	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("write saved credentials: %w", err)
	}

	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync saved credentials: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close saved credentials: %w", err)
	}

	return nil
}

func (store credentialStore) remove() error {
	err := os.Remove(store.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	if err != nil {
		return fmt.Errorf("remove credential file: %w", err)
	}

	return nil
}

func writeProtectedExport(path string, credentials storedCredentials) error {
	//nolint:gosec // Explicit export is requested by the caller and the file is created mode 0600.
	data, err := json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return fmt.Errorf("encode credential export: %w", err)
	}

	// This explicit path is supplied by the caller as a local export destination.
	//nolint:gosec // Export intentionally creates the caller-selected new file.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, privateFileMode)
	if err != nil {
		return fmt.Errorf("create new credential export file: %w", err)
	}

	defer func() { _ = file.Close() }()

	err = file.Chmod(privateFileMode)
	if err != nil {
		return fmt.Errorf("set export file permissions: %w", err)
	}

	err = restrictCredentialPath(path)
	if err != nil {
		return fmt.Errorf("protect export file: %w", err)
	}

	_, err = file.Write(append(data, '\n'))
	if err != nil {
		return fmt.Errorf("write credential export: %w", err)
	}

	err = file.Sync()
	if err != nil {
		return fmt.Errorf("sync credential export: %w", err)
	}

	err = file.Close()
	if err != nil {
		return fmt.Errorf("close credential export: %w", err)
	}

	return nil
}

func safeOperationError(operation string, err error, secrets ...string) error {
	if errors.Is(err, context.Canceled) {
		return operationError{operation: operation, message: errRequestCanceled.Error()}
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return operationError{operation: operation, message: errRequestDeadlineExceeded.Error()}
	}

	message := err.Error()

	for _, secret := range secrets {
		if secret == "" {
			continue
		}

		message = strings.ReplaceAll(message, secret, "[REDACTED]")
		message = strings.ReplaceAll(message, url.QueryEscape(secret), "[REDACTED]")
		message = strings.ReplaceAll(message, url.PathEscape(secret), "[REDACTED]")
	}

	message = tokenQueryPattern.ReplaceAllString(message, "${1}[REDACTED]")

	return operationError{operation: operation, message: message}
}

func isTerminalReader(reader io.Reader) bool {
	file, ok := reader.(*os.File)

	return ok && term.IsTerminal(int(file.Fd()))
}
