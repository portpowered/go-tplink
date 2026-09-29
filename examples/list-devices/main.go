// Package main lists devices from a TP-Link Kasa Cloud account.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/portpowered/go-tplink/pkg/tplink"
	"github.com/portpowered/go-tplink/pkg/tplinkmodels"
)

const loginTimeout = 20 * time.Second

var (
	errMissingCredentials  = errors.New("TP_LINK_EMAIL and TP_LINK_PASSWORD are required")
	errCredentialsRejected = errors.New("TP-Link rejected the supplied credentials")
	errSessionExpired      = errors.New("TP-Link session expired; run the example again to sign in")
)

func main() {
	err := run()
	if err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	email := os.Getenv("TP_LINK_EMAIL")
	password := os.Getenv("TP_LINK_PASSWORD")

	if email == "" || password == "" {
		return errMissingCredentials
	}

	client, err := newClientFromEnvironment()
	if err != nil {
		return err
	}

	defer func() {
		err := client.Close()
		if err != nil {
			log.Printf("close TP-Link client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), loginTimeout)
	defer cancel()

	return listAndPrintDevices(ctx, client, email, password)
}

func newClientFromEnvironment() (*tplink.Client, error) {
	var options []tplink.Option
	if baseURL := os.Getenv("TP_LINK_BASE_URL"); baseURL != "" {
		options = append(options, tplink.WithBaseURL(baseURL))
	}

	client, err := tplink.NewClient(options...)
	if err != nil {
		return nil, fmt.Errorf("create TP-Link client: %w", err)
	}

	return client, nil
}

func listAndPrintDevices(ctx context.Context, client *tplink.Client, email, password string) error {
	session, err := client.Login(ctx, tplink.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		var authErr *tplinkmodels.AuthenticationError
		if errors.As(err, &authErr) {
			return errCredentialsRejected
		}

		return fmt.Errorf("TP-Link login failed: %w", err)
	}

	result, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: session.Token},
	})
	if err != nil {
		var expired *tplinkmodels.TokenExpiredError
		if errors.As(err, &expired) {
			return errSessionExpired
		}

		return fmt.Errorf("list TP-Link devices: %w", err)
	}

	for _, device := range result.Devices {
		fmt.Printf("%s\t%s\t%s\n", device.DeviceType, device.DeviceModel, device.Alias)
	}

	return nil
}
