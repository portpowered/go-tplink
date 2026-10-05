// Command replaycoverage measures replay coverage across public, projection,
// and HTTP transport code, excluding generated files.
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
	minimum                 = 80.0
	target                  = 90.0
	coveragePercentageScale = 100.0
	coverageProfileFields   = 3
	coverageCommandName     = "go"
)

const productionPackages = "github.com/portpowered/go-tplink/pkg/tplink " +
	"github.com/portpowered/go-tplink/pkg/tplinkmodels " +
	"github.com/portpowered/go-tplink/pkg/dependencies/cloud"

var (
	errCoverageBelowMinimum = errors.New("replay coverage is below the minimum")
	errInvalidCoverageLine  = errors.New("invalid Go coverage profile record")
	errNoCoverageStatements = errors.New("coverage profile has no production statements")
	errConflictingCoverage  = errors.New("duplicate coverage records disagree")
)

type coverageTotals struct {
	covered int
	total   int
}

type coverageBlock struct {
	packageName string
	statements  int
	generated   bool
	covered     bool
}

func main() {
	err := run()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run() error {
	packages := strings.Fields(productionPackages)

	tempDir, err := os.MkdirTemp("", "go-tplink-replay-coverage-")
	if err != nil {
		return fmt.Errorf("create temporary coverage directory: %w", err)
	}

	defer func() { _ = os.RemoveAll(tempDir) }()

	profile := filepath.Join(tempDir, "coverage.out")
	command := newCoverageCommand(profile, packages)

	err = command.Run()
	if err != nil {
		return fmt.Errorf("run replay coverage tests: %w", err)
	}

	coverage, generatedFiles, err := measureCoverage(profile, packages)
	if err != nil {
		return err
	}

	combined := coverageTotals{covered: 0, total: 0}

	for _, packageName := range packages {
		totals := coverage[packageName]
		combined.covered += totals.covered
		combined.total += totals.total
		percentage := coveragePercentage(totals)
		fmt.Printf("%s replay coverage: %.1f%% (%d/%d statements)\n", packageName, percentage, totals.covered, totals.total)
	}

	percentage := coveragePercentage(combined)
	fmt.Printf(
		"combined non-generated production replay coverage: %.1f%% (%d/%d statements); minimum %.0f%%; target %.0f%%\n",
		percentage, combined.covered, combined.total, minimum, target,
	)
	fmt.Printf("Excluded generated files from the measured population: %d\n", generatedFiles)

	if percentage < minimum {
		return fmt.Errorf(
			"combined non-generated production replay coverage %.1f%% is below the %.0f%% minimum: %w",
			percentage,
			minimum,
			errCoverageBelowMinimum,
		)
	}

	return nil
}

func newCoverageCommand(profile string, packages []string) *exec.Cmd {
	//nolint:gosec // The command and arguments are fixed; profile is a private temporary output path.
	command := exec.CommandContext(
		context.Background(),
		coverageCommandName,
		"test",
		"-count=1",
		"-covermode=set",
		"-coverpkg="+strings.Join(packages, ","),
		"-coverprofile="+profile,
		"./tests/replay",
		"./pkg/tplinkmodels",
		"./pkg/dependencies/cloud",
	)
	command.Stdout = os.Stdout

	command.Stderr = os.Stderr

	return command
}

func measureCoverage(profile string, packageNames []string) (map[string]coverageTotals, int, error) {
	coverage := make(map[string]coverageTotals, len(packageNames))
	for _, packageName := range packageNames {
		coverage[packageName] = coverageTotals{covered: 0, total: 0}
	}

	generatedFiles := make(map[string]bool)
	blocks := make(map[string]coverageBlock)

	//nolint:gosec // The profile path is created in this command's private temporary directory.
	file, err := os.Open(profile)
	if err != nil {
		return nil, 0, fmt.Errorf("open Go coverage profile: %w", err)
	}

	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		err := addCoverageBlock(scanner.Text(), packageNames, blocks, generatedFiles)
		if err != nil {
			return nil, 0, err
		}
	}

	err = scanner.Err()
	if err != nil {
		return nil, 0, fmt.Errorf("read Go coverage profile: %w", err)
	}

	coverage = aggregateCoverage(blocks, coverage)

	for packageName, totals := range coverage {
		if totals.total == 0 {
			return nil, len(generatedFiles), fmt.Errorf(
				"coverage profile contains no statements for %s: %w",
				packageName,
				errNoCoverageStatements,
			)
		}
	}

	return coverage, len(generatedFiles), nil
}

func addCoverageBlock(
	line string,
	packageNames []string,
	blocks map[string]coverageBlock,
	generatedFiles map[string]bool,
) error {
	packageName, lineCovered, lineTotal, generated, err := parseCoverageRecord(line, packageNames)
	if err != nil || packageName == "" {
		return err
	}

	if generated {
		generatedFiles[coverageFile(line)] = true
	}

	fields := strings.Fields(line)
	key := fields[0]

	block, found := blocks[key]

	if found {
		if block.packageName != packageName || block.statements != lineTotal || block.generated != generated {
			return fmt.Errorf("conflicting duplicate coverage record %q: %w", key, errConflictingCoverage)
		}

		block.covered = block.covered || lineCovered > 0
		blocks[key] = block

		return nil
	}

	blocks[key] = coverageBlock{
		packageName: packageName,
		statements:  lineTotal,
		generated:   generated,
		covered:     lineCovered > 0,
	}

	return nil
}

func aggregateCoverage(blocks map[string]coverageBlock, coverage map[string]coverageTotals) map[string]coverageTotals {
	for _, block := range blocks {
		if block.generated {
			continue
		}

		totals := coverage[block.packageName]
		totals.total += block.statements

		if block.covered {
			totals.covered += block.statements
		}

		coverage[block.packageName] = totals
	}

	return coverage
}

func parseCoverageRecord(line string, packageNames []string) (string, int, int, bool, error) {
	if strings.HasPrefix(line, "mode:") {
		return "", 0, 0, false, nil
	}

	fields := strings.Fields(line)
	if len(fields) != coverageProfileFields {
		return "", 0, 0, false, fmt.Errorf("invalid Go coverage profile record %q: %w", line, errInvalidCoverageLine)
	}

	packageName := ""

	for _, candidate := range packageNames {
		if strings.HasPrefix(fields[0], candidate+"/") {
			packageName = candidate

			break
		}
	}

	if packageName == "" {
		return "", 0, 0, false, nil
	}

	generated := strings.HasSuffix(coverageFile(line), ".gen.go")

	statements, err := strconv.Atoi(fields[1])
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("parse statement count in coverage record %q: %w", line, err)
	}

	count, err := strconv.Atoi(fields[2])
	if err != nil {
		return "", 0, 0, false, fmt.Errorf("parse execution count in coverage record %q: %w", line, err)
	}

	if count > 0 {
		return packageName, statements, statements, generated, nil
	}

	return packageName, 0, statements, generated, nil
}

func coverageFile(line string) string {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return ""
	}

	file, _, found := strings.Cut(fields[0], ":")
	if !found {
		return ""
	}

	return filepath.ToSlash(file)
}

func coveragePercentage(totals coverageTotals) float64 {
	if totals.total == 0 {
		return 0
	}

	return float64(totals.covered) / float64(totals.total) * coveragePercentageScale
}
