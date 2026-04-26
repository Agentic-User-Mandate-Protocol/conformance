from pathlib import Path

from aump_conformance.resources import bundled_fixtures_path
from aump_conformance.runner import run_suite


def test_fixture_suite_passes() -> None:
    report = run_suite(Path("fixtures"))
    assert report.failed == 0


def test_bundled_fixture_suite_passes() -> None:
    with bundled_fixtures_path() as fixtures:
        report = run_suite(fixtures)
    assert report.failed == 0
