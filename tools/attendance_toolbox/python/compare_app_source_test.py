# -*- coding: utf-8 -*-
"""Unit tests for compare_app_source strict adapter canonicalization."""
from __future__ import annotations

import importlib.util
import tempfile
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "scripts" / "compare_app_source.py"


def _load_compare():
    spec = importlib.util.spec_from_file_location("compare_app_source", SCRIPT)
    mod = importlib.util.module_from_spec(spec)
    assert spec and spec.loader
    spec.loader.exec_module(mod)
    return mod


class CompareAppSourceTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.mod = _load_compare()

    def test_pure_newline_diff_is_equal(self) -> None:
        app = b"a = 1\r\nb = 2\r\n"
        tbx = b"a = 1\nb = 2\n"
        self.assertEqual(
            self.mod.classify_difference("leave/calc_leave.py", app, tbx),
            "equal",
        )

    def test_real_base_block_and_compat_call_adapter_only(self) -> None:
        """Matches the real D:\\app vs toolbox adapter shape (_BASE block)."""
        app = (
            "_BASE = os.path.dirname(_HERE)\n"
            "if _HERE not in sys.path:\n"
            "    sys.path.insert(0, _HERE)\n"
            "import calc_leave  # noqa: E402\n"
            "\n"
            "def parse(path):\n"
            "    wb = openpyxl.load_workbook(path, data_only=True)\n"
            "    return wb\n"
        ).encode()
        tbx = (
            "_BASE = os.path.dirname(_HERE)\n"
            "if _BASE not in sys.path:\n"
            "    sys.path.insert(0, _BASE)\n"
            "if _HERE not in sys.path:\n"
            "    sys.path.insert(0, _HERE)\n"
            "import calc_leave  # noqa: E402\n"
            "from excel_compat import load_workbook_compat  # noqa: E402\n"
            "\n"
            "def parse(path):\n"
            "    wb = load_workbook_compat(path, data_only=True)\n"
            "    return wb\n"
        ).encode()
        self.assertEqual(
            self.mod.classify_difference("overtime/fill_overtime_fields.py", app, tbx),
            "adapter_only",
        )
        self.assertEqual(
            self.mod.classify_difference("finally/calc_finally.py", app, tbx),
            "adapter_only",
        )

    def test_impure_compat_call_with_business_expression_fails(self) -> None:
        app = b"wb = openpyxl.load_workbook(path)\n"
        tbx = b"wb = load_workbook_compat(path); score = x * 999\n"
        self.assertEqual(
            self.mod.classify_difference("overtime/fill_overtime_fields.py", app, tbx),
            "business_divergence",
        )

    def test_extra_business_formula_on_allowlist_file_fails(self) -> None:
        app = (
            "def score(x):\n"
            "    return x * 2\n"
            "wb = openpyxl.load_workbook(path)\n"
        ).encode()
        tbx = (
            "if _BASE not in sys.path:\n"
            "    sys.path.insert(0, _BASE)\n"
            "from excel_compat import load_workbook_compat\n"
            "def score(x):\n"
            "    return x * 3\n"
            "wb = load_workbook_compat(path)\n"
        ).encode()
        self.assertEqual(
            self.mod.classify_difference("overtime/fill_overtime_fields.py", app, tbx),
            "business_divergence",
        )

    def test_adapter_on_non_allowlist_file_fails(self) -> None:
        app = b"wb = openpyxl.load_workbook(path)\n"
        tbx = (
            "if _BASE not in sys.path:\n"
            "    sys.path.insert(0, _BASE)\n"
            "from excel_compat import load_workbook_compat\n"
            "wb = load_workbook_compat(path)\n"
        ).encode()
        # leave is not in adapter allowlist
        self.assertEqual(
            self.mod.classify_difference("leave/calc_leave.py", app, tbx),
            "business_divergence",
        )

    def test_manifest_generation_roundtrip(self) -> None:
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            app = root / "app"
            tbx = root / "tbx"
            for rel in self.mod.PAIRS:
                (app / rel).parent.mkdir(parents=True, exist_ok=True)
                (tbx / rel).parent.mkdir(parents=True, exist_ok=True)
                body = f"# {rel}\nVALUE = 1\n"
                (app / rel).write_text(body, encoding="utf-8", newline="\n")
                (tbx / rel).write_text(body, encoding="utf-8", newline="\n")

            target = "overtime/fill_overtime_fields.py"
            app_body = (app / target).read_text(encoding="utf-8")
            (tbx / target).write_text(
                "if _BASE not in sys.path:\n"
                "    sys.path.insert(0, _BASE)\n"
                "from excel_compat import load_workbook_compat  # noqa: E402\n"
                + app_body.replace(
                    "VALUE = 1",
                    "wb = load_workbook_compat('x')\nVALUE = 1",
                )
                if "VALUE = 1" in app_body
                else (
                    "if _BASE not in sys.path:\n"
                    "    sys.path.insert(0, _BASE)\n"
                    "from excel_compat import load_workbook_compat  # noqa: E402\n"
                    + app_body
                ),
                encoding="utf-8",
            )
            # Ensure app has the openpyxl form if we injected a call
            app_text = (app / target).read_text(encoding="utf-8")
            if "load_workbook_compat" not in app_text and "VALUE = 1" in app_text:
                (app / target).write_text(
                    app_text.replace("VALUE = 1", "wb = openpyxl.load_workbook('x')\nVALUE = 1"),
                    encoding="utf-8",
                )
                tbx_text = (tbx / target).read_text(encoding="utf-8")
                if "load_workbook_compat('x')" not in tbx_text:
                    (tbx / target).write_text(
                        "if _BASE not in sys.path:\n"
                        "    sys.path.insert(0, _BASE)\n"
                        "from excel_compat import load_workbook_compat  # noqa: E402\n"
                        + app_text.replace(
                            "VALUE = 1",
                            "wb = load_workbook_compat('x')\nVALUE = 1",
                        ),
                        encoding="utf-8",
                    )

            # Simpler deterministic fixture for the allowlisted pair
            core = "def f():\n    return 1\n"
            (app / target).write_text(
                core + "wb = openpyxl.load_workbook(path)\n", encoding="utf-8"
            )
            (tbx / target).write_text(
                "if _BASE not in sys.path:\n"
                "    sys.path.insert(0, _BASE)\n"
                "from excel_compat import load_workbook_compat  # noqa: E402\n"
                + core
                + "wb = load_workbook_compat(path)\n",
                encoding="utf-8",
            )

            manifest = self.mod.build_manifest(app, tbx)
            self.assertTrue(manifest["_ok"], manifest)
            kinds = {f["path"]: f["difference_kind"] for f in manifest["files"]}
            self.assertEqual(kinds[target], "adapter_only")
            self.assertIn("difference_kind", manifest["files"][0])
            self.assertNotIn("app_path", manifest["files"][0])
            self.assertEqual(manifest["adapter_only_count"], 1)
            self.assertEqual(manifest["business_divergence_count"], 0)


if __name__ == "__main__":
    unittest.main()
