//go:build !windows

package main

import "os"

func restrictCredentialPath(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	mode := os.FileMode(privateFileMode)
	if info.IsDir() {
		mode = privateDirMode
	}

	return os.Chmod(path, mode)
}

func replaceCredentialFile(source, destination string) error {
	return os.Rename(source, destination)
}
