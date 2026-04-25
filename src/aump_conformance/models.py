"""Shared report models for the AUMP conformance runner."""

from __future__ import annotations

from dataclasses import dataclass, field
from typing import Any


@dataclass(frozen=True)
class CaseResult:
    """Result for a single conformance case."""

    id: str
    category: str
    title: str
    passed: bool
    expected: Any
    actual: Any
    message: str = ""
    reason_codes: list[str] = field(default_factory=list)
    paths: list[str] = field(default_factory=list)
    details: dict[str, Any] = field(default_factory=dict)

    def to_dict(self) -> dict[str, Any]:
        """Serialize to JSON-compatible data."""
        return {
            "id": self.id,
            "category": self.category,
            "title": self.title,
            "passed": self.passed,
            "expected": self.expected,
            "actual": self.actual,
            "message": self.message,
            "reason_codes": self.reason_codes,
            "paths": self.paths,
            "details": self.details,
        }


@dataclass(frozen=True)
class SuiteReport:
    """Aggregate report for a conformance suite run."""

    suite: str
    version: str
    spec_version: str
    results: list[CaseResult]

    @property
    def total(self) -> int:
        """Total case count."""
        return len(self.results)

    @property
    def passed(self) -> int:
        """Passed case count."""
        return sum(1 for result in self.results if result.passed)

    @property
    def failed(self) -> int:
        """Failed case count."""
        return self.total - self.passed

    def to_dict(self) -> dict[str, Any]:
        """Serialize to JSON-compatible data."""
        return {
            "suite": self.suite,
            "version": self.version,
            "spec_version": self.spec_version,
            "summary": {
                "total": self.total,
                "passed": self.passed,
                "failed": self.failed,
            },
            "results": [result.to_dict() for result in self.results],
        }
