"""Tests for scripts/check_public_disclosure.py.

Run with: python3 -m unittest discover -s scripts -p 'test_*.py'
"""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path

SCRIPTS = Path(__file__).resolve().parent
_spec = importlib.util.spec_from_file_location("check_public_disclosure", SCRIPTS / "check_public_disclosure.py")
assert _spec and _spec.loader
cpd = importlib.util.module_from_spec(_spec)
sys.modules["check_public_disclosure"] = cpd
_spec.loader.exec_module(cpd)

# Built at run time, so this file holds no ticket reference itself.
TICKET = "POLY" + "-" + "1234"


class TicketReferenceTest(unittest.TestCase):
    def check(self, files: dict[str, bytes]) -> list[str]:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            for name, content in files.items():
                path = root / name
                path.parent.mkdir(parents=True, exist_ok=True)
                path.write_bytes(content)
            return cpd.ticket_references(root, files)

    def test_refuses_a_ticket_reference_in_a_source_file(self) -> None:
        failures = self.check({"services/orders.go": f"package services\n\n// {TICKET}: keep it.\n".encode()})
        self.assertEqual(failures, ["services/orders.go:3: contains an internal ticket ID"])

    def test_refuses_any_case(self) -> None:
        failures = self.check({"hardening/x_test.go": f'x := "{TICKET.lower()}-control"\n'.encode()})
        self.assertEqual(failures, ["hardening/x_test.go:1: contains an internal ticket ID"])

    def test_refuses_a_ticket_reference_in_a_path(self) -> None:
        name = f"codecs/{TICKET.lower()}_test.go"
        self.assertEqual(self.check({name: b"package codecs\n"}), [f"{name}: path contains an internal ticket ID"])

    def test_ignores_generated_code_and_binary_files(self) -> None:
        failures = self.check({
            "gen/orders/v1/orders.pb.go": f"// {TICKET}\n".encode(),
            "testdata/blob.bin": b"\xff\xfe" + TICKET.encode(),
        })
        self.assertEqual(failures, [])

    def test_accepts_text_without_a_ticket_reference(self) -> None:
        failures = self.check({
            "README.md": b"Polyester SDK, POLYESTER_API_URL, poly-market and POLY- alone.\n",
            "scripts/check.py": b'TICKET = re.compile(r"(?<![A-Za-z0-9])poly-\\d+(?!\\d)")\n',
        })
        self.assertEqual(failures, [])

    def test_the_repository_is_clean(self) -> None:
        root = SCRIPTS.parent
        self.assertEqual(cpd.ticket_references(root, cpd.tracked_files(root)), [])


if __name__ == "__main__":
    unittest.main()
