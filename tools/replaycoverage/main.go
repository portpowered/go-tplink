// Command replaycoverage runs the offline replay tests with coverage enabled
// for pkg/tplink and enforces the minimum supported coverage level.
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

const (
	clientPackage           = "github.com/portpowered/go-tplink/pkg/tplink"
	minimum                 = 90.0
	coveragePercentageScale = 100.0
	coverageProfileFields   = 3
	coverageCommandName     = "go"
)

var (
	errCoverageBelowMinimum = errors.New("replay coverage is below the minimum")
	errInvalidCoverageLine  = errors.New("invalid Go coverage profile record")
	errNoCoverageStatements = errors.New("coverage profile has no client statements")
)

func main() {
	err := run()
	if err != nil {
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
	//nolint:gosec // The command and arguments are fixed; profile is a private temporary output path.
	command := exec.CommandContext(
		context.Background(),
		coverageCommandName,
		"test",
		"-count=1",
		"-covermode=set",
		"-coverpkg="+clientPackage,
		"-coverprofile="+profile,
		"./tests/replay",
	)
	command.Stdout = os.Stdout

	command.Stderr = os.Stderr

	err = command.Run()
	if err != nil {
		return fmt.Errorf("run replay coverage tests: %w", err)
	}

	covered, total, err := measureCoverage(profile)
	if err != nil {
		return err
	}

	percentage := float64(covered) / float64(total) * coveragePercentageScale
	fmt.Printf(
		"pkg/tplink replay coverage: %.1f%% (%d/%d statements); minimum %.0f%%\n",
		percentage,
		covered,
		total,
		minimum,
	)

	if percentage < minimum {
		return fmt.Errorf(
			"pkg/tplink replay coverage %.1f%% is below the %.0f%% minimum: %w",
			percentage,
			minimum,
			errCoverageBelowMinimum,
		)
	}

	return nil
}

func measureCoverage(profile string) (int, int, error) {
	covered := 0
	total := 0
	//nolint:gosec // The profile path is created in this command's private temporary directory.
	file, err := os.Open(profile)
	if err != nil {
		return 0, 0, fmt.Errorf("open Go coverage profile: %w", err)
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		lineCovered, lineTotal, parseErr := parseCoverageRecord(scanner.Text())
		if parseErr != nil {
			return 0, 0, parseErr
		}

		covered += lineCovered
		total += lineTotal
	}

	err = scanner.Err()
	if err != nil {
		return 0, 0, fmt.Errorf("read Go coverage profile: %w", err)
	}

	if total == 0 {
		return 0, 0, fmt.Errorf(
			"coverage profile contains no statements for %s: %w",
			clientPackage,
			errNoCoverageStatements,
		)
	}

	return covered, total, nil
}

func parseCoverageRecord(line string) (int, int, error) {
	if strings.HasPrefix(line, "mode:") {
		return 0, 0, nil
	}

	fields := strings.Fields(line)
	if len(fields) != coverageProfileFields {
		return 0, 0, fmt.Errorf("invalid Go coverage profile record %q: %w", line, errInvalidCoverageLine)
	}

	if !strings.HasPrefix(fields[0], clientPackage+"/") {
		return 0, 0, nil
	}

	statements, err := strconv.Atoi(fields[1])
	if err != nil {
		return 0, 0, fmt.Errorf("parse statement count in coverage record %q: %w", line, err)
	}

	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return 0, 0, fmt.Errorf("parse execution count in coverage record %q: %w", line, err)
	}

	if count > 0 {
		return statements, statements, nil
	}

	return 0, statements, nil
}
