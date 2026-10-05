#!/usr/bin/env python3
"""Generate combined non-generated replay coverage assets for the docs site."""

from __future__ import annotations

import argparse
import json
import subprocess
import tempfile
from collections import defaultdict
from pathlib import Path


PRODUCTION_PACKAGES = (
    "github.com/portpowered/go-tplink/pkg/tplink",
    "github.com/portpowered/go-tplink/pkg/tplinkmodels",
    "github.com/portpowered/go-tplink/pkg/dependencies/cloud",
)
MINIMUM_COVERAGE = 80.0
TARGET_COVERAGE = 90.0


def coverage_profile(
    source: Path, filtered: Path
) -> tuple[dict[str, tuple[int, int]], set[str]]:
    totals: dict[str, list[int]] = defaultdict(lambda: [0, 0])
    generated_files: set[str] = set()
    filtered_lines: list[str] = []
    blocks: dict[str, tuple[str, int, bool, bool]] = {}

    for line in source.read_text(encoding="utf-8").splitlines():
        if line.startswith("mode:"):
            filtered_lines.append(line)
            continue

        fields = line.split()
        if len(fields) != 3:
            raise RuntimeError(f"invalid Go coverage profile record: {line!r}")

        filename = fields[0].split(":", maxsplit=1)[0].replace("\\", "/")
        package = next(
            (candidate for candidate in PRODUCTION_PACKAGES if filename.startswith(candidate + "/")),
            None,
        )
        if package is None:
            continue

        generated = filename.endswith(".gen.go")
        statements = int(fields[1])
        executions = int(fields[2])
        if statements < 0 or executions < 0:
            raise RuntimeError(f"negative Go coverage profile count: {line!r}")
        key = fields[0]
        covered = executions > 0
        previous = blocks.get(key)
        if previous is not None:
            previous_package, previous_statements, previous_generated, previous_covered = previous
            if (package, statements, generated) != (previous_package, previous_statements, previous_generated):
                raise RuntimeError(f"conflicting duplicate coverage record: {key!r}")
            covered = covered or previous_covered
        blocks[key] = (package, statements, generated, covered)

    for key, (package, statements, generated, covered) in blocks.items():
        if generated:
            generated_files.add(key.split(":", maxsplit=1)[0].replace("\\", "/"))
            continue
        totals[package][0] += statements if covered else 0
        totals[package][1] += statements
        filtered_lines.append(f"{key} {statements} {int(covered)}")

    missing = set(PRODUCTION_PACKAGES) - totals.keys()
    if missing:
        raise RuntimeError(f"coverage profile has no non-generated statements for {sorted(missing)}")

    filtered.write_text("\n".join(filtered_lines) + "\n", encoding="utf-8")
    return {package: (values[0], values[1]) for package, values in totals.items()}, generated_files


def percentage(covered: int, total: int) -> float:
    if total == 0:
        return 0.0
    return covered / total * 100.0


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument(
        "--site-dir",
        type=Path,
        default=Path(__file__).resolve().parents[1] / "site",
        help="directory where coverage.json and coverage.html will be written",
    )
    args = parser.parse_args()
    site_dir = args.site_dir.resolve()

    root = Path(__file__).resolve().parents[1]
    site_dir.mkdir(parents=True, exist_ok=True)

    with tempfile.TemporaryDirectory(prefix="go-tplink-replay-coverage-") as temp_dir:
        temp_path = Path(temp_dir)
        profile = temp_path / "coverage.replay.out"
        filtered_profile = temp_path / "coverage.non-generated.out"
        subprocess.run(
            [
                "go",
                "test",
                "-count=1",
                "-covermode=set",
                f"-coverpkg={','.join(PRODUCTION_PACKAGES)}",
                f"-coverprofile={profile}",
                "./tests/replay",
                "./pkg/tplinkmodels",
                "./pkg/dependencies/cloud",
            ],
            cwd=root,
            check=True,
        )

        by_package, generated_files = coverage_profile(profile, filtered_profile)
        combined_covered = sum(covered for covered, _ in by_package.values())
        combined_total = sum(total for _, total in by_package.values())
        combined_percentage = percentage(combined_covered, combined_total)

        for package in PRODUCTION_PACKAGES:
            covered, total = by_package[package]
            print(f"{package} replay coverage: {percentage(covered, total):.1f}% ({covered}/{total})")

        print(
            "combined non-generated production replay coverage: "
            f"{combined_percentage:.1f}% ({combined_covered}/{combined_total}); "
            f"minimum {MINIMUM_COVERAGE:.0f}%; target {TARGET_COVERAGE:.0f}%"
        )
        print(f"Excluded generated files from the measured population: {len(generated_files)}")

        subprocess.run(
            ["go", "tool", "cover", f"-html={filtered_profile}", f"-o={site_dir / 'coverage.html'}"],
            cwd=root,
            check=True,
        )
        color = "brightgreen" if combined_percentage >= 95 else "green" if combined_percentage >= 90 else "red"
        badge = {
            "schemaVersion": 1,
            "label": "replay coverage",
            "message": f"{combined_percentage:.1f}%",
            "color": color,
        }
        (site_dir / "coverage.json").write_text(
            json.dumps(badge, indent=2) + "\n", encoding="utf-8"
        )

    if combined_percentage < MINIMUM_COVERAGE:
        raise SystemExit(
            "combined non-generated production replay coverage "
            f"{combined_percentage:.1f}% is below the {MINIMUM_COVERAGE:.0f}% minimum"
        )
    print(f"Wrote {site_dir / 'coverage.json'} and {site_dir / 'coverage.html'}")

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
