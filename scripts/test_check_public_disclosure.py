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

    def test_refuses_every_numbered_form(self) -> None:
        for text in ("func Test" + "Ticket" + "8581Depth(t *testing.T) {}", "func Test" + "Poly" + "9301Market() {}",
                     'id := "' + "poly" + '4996-wd"', "POLY" + "_1234",
                     # Short numbers are tickets too, hyphenated or not.
                     "see " + "POLY" + "-12", "see " + "Ticket" + "_42", "Poly" + "7",
                     # Only Poly1305 itself is excluded: other numbers that
                     # start with 1305, and its hyphenated form, are tickets.
                     "Poly" + "13050", "POLY" + "-1305"):
            with self.subTest(text=text):
                self.assertEqual(self.check({"x_test.go": (text + "\n").encode()}),
                                 ["x_test.go:1: contains an internal ticket ID"])
        name = "services/orderbook_" + "ticket" + "8512_test.go"
        self.assertEqual(self.check({name: b"package services\n"}), [f"{name}: path contains an internal ticket ID"])

    def test_ignores_generated_code_and_binary_files(self) -> None:
        failures = self.check({
            "gen/orders/v1/orders.pb.go": f"// {TICKET}\n".encode(),
            "testdata/blob.bin": b"\xff\xfe" + TICKET.encode(),
            # Valid UTF-8, but binary as Git reads it: it holds a NUL byte.
            "testdata/image.bin": b"\x00" + TICKET.encode() + b"\x00",
        })
        self.assertEqual(failures, [])

    def test_a_binary_file_is_still_checked_by_its_path(self) -> None:
        name = "testdata/" + TICKET.lower() + ".bin"
        self.assertEqual(self.check({name: b"\x00\x01"}), [f"{name}: path contains an internal ticket ID"])

    def test_a_nul_byte_past_the_sniffed_prefix_leaves_a_text_file(self) -> None:
        content = b"a" * cpd.BINARY_SNIFF_BYTES + b"\n// " + TICKET.encode() + b"\n\x00"
        self.assertEqual(self.check({"big.txt": content}), ["big.txt:2: contains an internal ticket ID"])

    def test_accepts_text_without_a_ticket_reference(self) -> None:
        failures = self.check({
            "README.md": b"Polyester SDK, POLYESTER_API_URL, poly-market, POLY- alone, a setup ticket.\n",
            "go.sum": b"golang.org/x/crypto/chacha20" + b"poly" + b"1305 h1:abc=\n",
            "crypto.go": b"// Poly1305, XChaCha20-Poly1305 and golang.org/x/crypto/internal/poly1305\n"
                         b"poly1305.Sum(&tag, msg, &key)\nvar POLY1305 = 1\n",
            "internal/poly1305/sum.go": b"package poly1305\n",
            "scripts/check.py": b'TICKET = re.compile(r"(?<![A-Za-z0-9])poly-\\d+(?!\\d)")\n',
        })
        self.assertEqual(failures, [])

    def test_markdown_refuses_finding_ids_with_or_without_a_hyphen(self) -> None:
        for finding in ("F" + "-24", "F" + "001", "F" + "018"):
            with tempfile.TemporaryDirectory() as tmp:
                root = Path(tmp)
                (root / "README.md").write_text("ok\n", encoding="utf-8")
                (root / "CHANGELOG.md").write_text(f"- Tests for {finding}.\n", encoding="utf-8")
                self.assertEqual(cpd.markdown_disclosures(root), ["CHANGELOG.md: contains internal finding ID"], finding)

    def test_the_repository_is_clean(self) -> None:
        root = SCRIPTS.parent
        self.assertEqual(cpd.ticket_references(root, cpd.tracked_files(root)), [])
        self.assertEqual(cpd.markdown_disclosures(root), [])


if __name__ == "__main__":
    unittest.main()
