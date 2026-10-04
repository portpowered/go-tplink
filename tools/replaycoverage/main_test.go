package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestMeasureCoverageMergesRepeatedBlocksAcrossTestBinaries(t *testing.T) {
	t.Parallel()

	profile := filepath.Join(t.TempDir(), "coverage.out")
	contents := "mode: set\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.go:1.1,1.2 1 0\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.go:1.1,1.2 1 1\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.go:2.1,3.2 2 0\n" +
		"github.com/portpowered/go-tplink/pkg/tplinkmodels/errors.go:1.1,1.2 2 1\n" +
		"github.com/portpowered/go-tplink/pkg/tplinkmodels/errors.go:1.1,1.2 2 0\n" +
		"github.com/portpowered/go-tplink/pkg/dependencies/cloud/cloud.go:1.1,1.2 3 0\n" +
		"github.com/portpowered/go-tplink/pkg/dependencies/cloud/cloud.go:1.1,1.2 3 1\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.gen.go:1.1,1.2 5 0\n"

	err := os.WriteFile(profile, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	packages := []string{
		"github.com/portpowered/go-tplink/pkg/tplink",
		"github.com/portpowered/go-tplink/pkg/tplinkmodels",
		"github.com/portpowered/go-tplink/pkg/dependencies/cloud",
	}

	coverage, generatedFiles, err := measureCoverage(profile, packages)
	if err != nil {
		t.Fatal(err)
	}

	want := map[string]coverageTotals{
		packages[0]: {covered: 1, total: 3},
		packages[1]: {covered: 2, total: 2},
		packages[2]: {covered: 3, total: 3},
	}
	for packageName, totals := range want {
		if coverage[packageName] != totals {
			t.Errorf("coverage[%q] = %+v, want %+v", packageName, coverage[packageName], totals)
		}
	}

	if generatedFiles != 1 {
		t.Errorf("generated file count = %d, want 1", generatedFiles)
	}
}

func TestMeasureCoverageRejectsConflictingDuplicateBlocks(t *testing.T) {
	t.Parallel()

	profile := filepath.Join(t.TempDir(), "coverage.out")
	contents := "mode: set\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.go:1.1,1.2 1 0\n" +
		"github.com/portpowered/go-tplink/pkg/tplink/client.go:1.1,1.2 2 1\n" +
		"github.com/portpowered/go-tplink/pkg/tplinkmodels/errors.go:1.1,1.2 1 1\n" +
		"github.com/portpowered/go-tplink/pkg/dependencies/cloud/cloud.go:1.1,1.2 1 1\n"

	err := os.WriteFile(profile, []byte(contents), 0o600)
	if err != nil {
		t.Fatal(err)
	}

	packages := []string{
		"github.com/portpowered/go-tplink/pkg/tplink",
		"github.com/portpowered/go-tplink/pkg/tplinkmodels",
		"github.com/portpowered/go-tplink/pkg/dependencies/cloud",
	}

	_, _, err = measureCoverage(profile, packages)
	if err == nil {
		t.Fatal("measureCoverage() error = nil, want conflicting duplicate error")
	}
}
