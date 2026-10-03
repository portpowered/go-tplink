//go:build !windows

package main

import (
	"fmt"
	"os"
)

func restrictCredentialPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("inspect credential path: %w", err)
	}

	mode := os.FileMode(privateFileMode)
	if info.IsDir() {
		mode = privateDirMode
	}

	err = os.Chmod(path, mode)
	if err != nil {
		return fmt.Errorf("restrict credential permissions: %w", err)
	}

	return nil
}

func replaceCredentialFile(source, destination string) error {
	err := os.Rename(source, destination)
	if err != nil {
		return fmt.Errorf("replace credential file: %w", err)
	}

	return nil
}
