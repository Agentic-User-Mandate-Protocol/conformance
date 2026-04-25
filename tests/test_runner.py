from pathlib import Path

from aump_conformance.runner import run_suite


def test_fixture_suite_passes() -> None:
    report = run_suite(Path("fixtures"))
    assert report.failed == 0
