"""Packaged conformance resources."""

from __future__ import annotations

from collections.abc import Iterator
from contextlib import contextmanager
from importlib.resources import as_file, files
from pathlib import Path


@contextmanager
def bundled_fixtures_path() -> Iterator[Path]:
    """Yield the packaged fixture corpus path."""
    packaged = files("aump_conformance").joinpath("fixtures")
    if not packaged.is_dir():
        yield Path(__file__).resolve().parents[2] / "fixtures"
        return
    with as_file(packaged) as path:
        yield path
