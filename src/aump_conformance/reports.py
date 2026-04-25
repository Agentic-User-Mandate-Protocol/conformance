"""Report rendering for AUMP conformance runs."""

from __future__ import annotations

from xml.etree import ElementTree

from aump_conformance.jsonio import dump_json
from aump_conformance.models import SuiteReport


def render_text(report: SuiteReport) -> str:
    """Render a compact terminal report."""
    lines = [
        f"{report.suite} v{report.version} (spec {report.spec_version})",
        f"{report.passed}/{report.total} passed",
    ]
    for result in report.results:
        status = "PASS" if result.passed else "FAIL"
        lines.append(f"{status} {result.id} - {result.title}")
        if not result.passed and result.message:
            lines.append(f"  {result.message}")
        if not result.passed:
            lines.append(f"  expected: {result.expected}")
            lines.append(f"  actual: {result.actual}")
    return "\n".join(lines) + "\n"


def render_json(report: SuiteReport) -> str:
    """Render a JSON report."""
    return dump_json(report.to_dict())


def render_junit(report: SuiteReport) -> str:
    """Render a JUnit XML report."""
    suite = ElementTree.Element(
        "testsuite",
        {
            "name": report.suite,
            "tests": str(report.total),
            "failures": str(report.failed),
        },
    )
    for result in report.results:
        case = ElementTree.SubElement(
            suite,
            "testcase",
            {
                "classname": result.category,
                "name": result.id,
            },
        )
        if not result.passed:
            failure = ElementTree.SubElement(
                case,
                "failure",
                {
                    "message": result.message or "conformance case failed",
                },
            )
            failure.text = dump_json(result.to_dict())

    return ElementTree.tostring(suite, encoding="unicode") + "\n"
