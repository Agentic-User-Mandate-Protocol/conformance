"""JSON Schema validation for AUMP conformance fixtures."""

from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path
from typing import Any

from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import ValidationError

from aump_conformance.jsonio import load_json

SCHEMA_FILES = {
    "mandate": "mandate.schema.json",
    "profile": "profile.schema.json",
    "action-evaluation": "action-evaluation.schema.json",
    "evidence-event": "evidence-event.schema.json",
}


@dataclass
class SchemaRegistry:
    """Loaded AUMP JSON schemas."""

    root: Path
    validators: dict[str, Draft202012Validator]

    @classmethod
    def load(cls, root: Path) -> SchemaRegistry:
        """Load schemas from a fixture root containing a schemas directory."""
        schema_dir = root / "schemas"
        validators: dict[str, Draft202012Validator] = {}
        for name, filename in SCHEMA_FILES.items():
            schema_path = schema_dir / filename
            schema = load_json(schema_path)
            Draft202012Validator.check_schema(schema)
            validators[name] = Draft202012Validator(
                schema,
                format_checker=FormatChecker(),
            )
        return cls(root=root, validators=validators)

    def validate(self, schema_name: str, payload: Any) -> list[str]:
        """Return validation error messages for a payload."""
        if schema_name not in self.validators:
            known = ", ".join(sorted(self.validators))
            raise ValueError(f"unknown schema {schema_name!r}; expected one of {known}")

        errors = sorted(
            self.validators[schema_name].iter_errors(payload),
            key=lambda error: list(error.absolute_path),
        )
        return [format_error(error) for error in errors]


def format_error(error: ValidationError) -> str:
    """Format a JSON Schema error with a stable JSON path."""
    path = "$"
    for part in error.absolute_path:
        if isinstance(part, int):
            path += f"[{part}]"
        else:
            path += f".{part}"
    return f"{path}: {error.message}"
