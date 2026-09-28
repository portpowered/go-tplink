#!/usr/bin/env python3
"""Generate replay-coverage assets for the API documentation site."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import tempfile
from pathlib import Path


CLIENT_PACKAGE = "github.com/portpowered/go-tplink/pkg/tplink"
MINIMUM_COVERAGE = 90.0
TOTAL_COVERAGE = re.compile(r"^total:\s+\(statements\)\s+([0-9.]+)%$", re.MULTILINE)


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
        profile = Path(temp_dir) / "coverage.replay.out"
        subprocess.run(
            [
                "go",
                "test",
                "-count=1",
                "-covermode=set",
                f"-coverpkg={CLIENT_PACKAGE}",
                f"-coverprofile={profile}",
                "./tests/replay",
            ],
            cwd=root,
            check=True,
        )
        summary = subprocess.run(
            ["go", "tool", "cover", f"-func={profile}"],
            cwd=root,
            check=True,
            capture_output=True,
            text=True,
        ).stdout
        match = TOTAL_COVERAGE.search(summary)
        if match is None:
            raise RuntimeError("go tool cover output did not contain total statement coverage")
        percentage = float(match.group(1))

        subprocess.run(
            ["go", "tool", "cover", f"-html={profile}", f"-o={site_dir / 'coverage.html'}"],
            cwd=root,
            check=True,
        )
        color = "brightgreen" if percentage >= 95 else "green" if percentage >= 90 else "red"
        badge = {
            "schemaVersion": 1,
            "label": "replay coverage",
            "message": f"{percentage:.1f}%",
            "color": color,
        }
        (site_dir / "coverage.json").write_text(
            json.dumps(badge, indent=2) + "\n", encoding="utf-8"
        )

    print(f"pkg/tplink replay coverage: {percentage:.1f}% (minimum {MINIMUM_COVERAGE:.0f}%)")
    if percentage < MINIMUM_COVERAGE:
        raise SystemExit(
            f"pkg/tplink replay coverage {percentage:.1f}% is below the "
            f"{MINIMUM_COVERAGE:.0f}% minimum"
        )
    print(f"Wrote {site_dir / 'coverage.json'} and {site_dir / 'coverage.html'}")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
