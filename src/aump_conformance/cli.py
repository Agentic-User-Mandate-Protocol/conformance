"""Command-line interface for the AUMP conformance runner."""

from __future__ import annotations

import argparse
import sys
from pathlib import Path

from aump_conformance.policy import parse_datetime
from aump_conformance.reports import render_json, render_junit, render_text
from aump_conformance.runner import run_suite


def main(argv: list[str] | None = None) -> int:
    """Run the CLI."""
    parser = argparse.ArgumentParser(
        prog="aump-conformance",
        description="Run the AUMP conformance suite.",
    )
    subparsers = parser.add_subparsers(dest="command", required=True)

    validate = subparsers.add_parser("validate", help="Run conformance fixtures.")
    validate.add_argument(
        "target",
        nargs="?",
        default="fixtures",
        help="Fixture directory or manifest JSON path.",
    )
    validate.add_argument(
        "--format",
        choices=("text", "json", "junit"),
        default="text",
        help="Report format.",
    )
    validate.add_argument("--output", help="Write report to this path.")
    validate.add_argument(
        "--now",
        help="Override fixture clock as an RFC 3339 timestamp.",
    )

    args = parser.parse_args(argv)
    if args.command == "validate":
        now = parse_datetime(args.now) if args.now else None
        report = run_suite(Path(args.target), now=now)
        rendered = _render(report, args.format)
        if args.output:
            Path(args.output).write_text(rendered, encoding="utf-8")
        else:
            sys.stdout.write(rendered)
        return 0 if report.failed == 0 else 1

    parser.error(f"unknown command {args.command}")
    return 2


def _render(report, report_format: str) -> str:
    if report_format == "json":
        return render_json(report)
    if report_format == "junit":
        return render_junit(report)
    return render_text(report)


if __name__ == "__main__":
    raise SystemExit(main())
