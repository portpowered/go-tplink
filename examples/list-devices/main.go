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

func main() {
	if err := run(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}

func run() error {
	email := os.Getenv("TP_LINK_EMAIL")
	password := os.Getenv("TP_LINK_PASSWORD")
	if email == "" || password == "" {
		return errors.New("TP_LINK_EMAIL and TP_LINK_PASSWORD are required")
	}

	var options []tplink.Option
	if baseURL := os.Getenv("TP_LINK_BASE_URL"); baseURL != "" {
		options = append(options, tplink.WithBaseURL(baseURL))
	}

	client, err := tplink.NewClient(options...)
	if err != nil {
		return fmt.Errorf("create TP-Link client: %w", err)
	}
	defer func() {
		if err := client.Close(); err != nil {
			log.Printf("close TP-Link client: %v", err)
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	session, err := client.Login(ctx, tplink.LoginRequest{
		Email:    email,
		Password: password,
	})
	if err != nil {
		var authErr *tplinkmodels.AuthenticationError
		if errors.As(err, &authErr) {
			return errors.New("TP-Link rejected the supplied credentials")
		}
		return fmt.Errorf("TP-Link login failed: %w", err)
	}

	result, err := client.GetDevices(ctx, tplink.GetDevicesRequest{
		Auth: tplink.AuthContext{AccessToken: session.Token},
	})
	if err != nil {
		var expired *tplinkmodels.TokenExpiredError
		if errors.As(err, &expired) {
			return errors.New("TP-Link session expired; run the example again to sign in")
		}
		return fmt.Errorf("list TP-Link devices: %w", err)
	}

	for _, device := range result.Devices {
		fmt.Printf("%s\t%s\t%s\n", device.DeviceType, device.DeviceModel, device.Alias)
	}
	return nil
}
