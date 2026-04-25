"""JSON file helpers."""

from __future__ import annotations

import json
from pathlib import Path
from typing import Any


def load_json(path: Path) -> Any:
    """Load JSON from a path with a useful error prefix."""
    try:
        with path.open(encoding="utf-8") as handle:
            return json.load(handle)
    except json.JSONDecodeError as exc:
        raise ValueError(f"{path}: invalid JSON: {exc}") from exc


def dump_json(data: Any) -> str:
    """Serialize stable, human-readable JSON."""
    return json.dumps(data, indent=2, sort_keys=True) + "\n"
