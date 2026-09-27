// Command replaycoverage runs the offline replay tests with coverage enabled
// for pkg/tplink and enforces the minimum supported coverage level.
package main

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	clientPackage = "github.com/portpowered/go-tplink/pkg/tplink"
	minimum       = 90.0
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	tempDir, err := os.MkdirTemp("", "go-tplink-replay-coverage-")
	if err != nil {
		return fmt.Errorf("create temporary coverage directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tempDir) }()

	profile := filepath.Join(tempDir, "coverage.out")
	command := exec.Command(
		"go",
		"test",
		"-count=1",
		"-covermode=set",
		"-coverpkg="+clientPackage,
		"-coverprofile="+profile,
		"./tests/replay",
	)
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("run replay coverage tests: %w", err)
	}

	covered, total, err := measureCoverage(profile)
	if err != nil {
		return err
	}
	percentage := float64(covered) / float64(total) * 100
	fmt.Printf("pkg/tplink replay coverage: %.1f%% (%d/%d statements); minimum %.0f%%\n", percentage, covered, total, minimum)
	if percentage < minimum {
		return fmt.Errorf("pkg/tplink replay coverage %.1f%% is below the %.0f%% minimum", percentage, minimum)
	}
	return nil
}

func measureCoverage(profile string) (covered, total int, err error) {
	file, err := os.Open(profile)
	if err != nil {
		return 0, 0, fmt.Errorf("open Go coverage profile: %w", err)
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}

		fields := strings.Fields(line)
		if len(fields) != 3 {
			return 0, 0, fmt.Errorf("invalid Go coverage profile record %q", line)
		}
		if !strings.HasPrefix(fields[0], clientPackage+"/") {
			continue
		}

		statements, parseErr := strconv.Atoi(fields[1])
		if parseErr != nil {
			return 0, 0, fmt.Errorf("parse statement count in coverage record %q: %w", line, parseErr)
		}
		count, parseErr := strconv.Atoi(fields[2])
		if parseErr != nil {
			return 0, 0, fmt.Errorf("parse execution count in coverage record %q: %w", line, parseErr)
		}
		total += statements
		if count > 0 {
			covered += statements
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, fmt.Errorf("read Go coverage profile: %w", err)
	}
	if total == 0 {
		return 0, 0, fmt.Errorf("Go coverage profile contains no statements for %s", clientPackage)
	}
	return covered, total, nil
}
