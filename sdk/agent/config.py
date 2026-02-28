from __future__ import annotations

import os
from dataclasses import dataclass, field
from pathlib import Path


@dataclass
class Config:
    """Runtime configuration for the mnemonic-agent."""

    mnemonic_binary: str = field(
        default_factory=lambda: os.environ.get(
            "MNEMONIC_BINARY",
            str(Path(__file__).resolve().parents[2] / "mnemonic"),
        )
    )
    mnemonic_config: str = field(
        default_factory=lambda: os.environ.get(
            "MNEMONIC_CONFIG",
            str(Path(__file__).resolve().parents[2] / "config.yaml"),
        )
    )
    project_cwd: str = field(default_factory=os.getcwd)
    model: str = field(
        default_factory=lambda: os.environ.get("MNEMONIC_AGENT_MODEL", "claude-sonnet-4-6")
    )
    permission_mode: str = "acceptEdits"
    evolve_interval: int = 5
    max_turns: int | None = None
    verbose: bool = False
    no_reflect: bool = False
    subagent_model: str = "sonnet"

    @property
    def project_root(self) -> str:
        """Project root derived from mnemonic config file location."""
        return str(Path(self.mnemonic_config).resolve().parent)

    @property
    def sdk_dir(self) -> str:
        return str(Path(__file__).resolve().parent)

    @property
    def evolution_dir(self) -> str:
        return str(Path(self.sdk_dir) / "evolution")
