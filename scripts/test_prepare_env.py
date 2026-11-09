import os
import re
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


class TestNFR04Environment(unittest.TestCase):
    def test_private_unique_env_and_no_overwrite(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "acceptance.env"
            command = [sys.executable, str(Path(__file__).with_name("prepare-env.py")),
                       "--out", str(output), "--project", "vkr-m5-acceptance",
                       "--db-port", "55432", "--backend-port", "18080"]
            result = subprocess.run(command, capture_output=True, check=True)
            values = dict(line.split("=", 1) for line in output.read_text().splitlines())
            self.assertEqual(os.stat(output).st_mode & 0o777, 0o600)
            password = values["POSTGRES_PASSWORD"]
            self.assertTrue(re.fullmatch("[0-9a-f]{64}", password))
            self.assertNotIn(password.encode(), result.stdout + result.stderr)
            self.assertIn("@db:5432/svoyshef?", values["DATABASE_URL"])
            self.assertIn("@127.0.0.1:55432/m5_integration?", values["M2_IT_DATABASE_URL"])
            self.assertNotEqual(values["LOCAL_DATABASE_URL"], values["M2_IT_DATABASE_URL"])
            original = output.read_bytes()
            result = subprocess.run(command, capture_output=True)
            self.assertEqual(result.returncode, 1)
            self.assertEqual(original, output.read_bytes())

    def test_invalid_inputs_do_not_create_env(self):
        with tempfile.TemporaryDirectory() as tmp:
            output = Path(tmp) / "invalid.env"
            for args in (["--project", "../bad"], ["--db-port", "0"],
                         ["--backend-port", "65536"], ["--db-port", "8080"]):
                result = subprocess.run(
                    [sys.executable, str(Path(__file__).with_name("prepare-env.py")),
                     "--out", str(output), *args], capture_output=True)
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output.exists())


if __name__ == "__main__":
    unittest.main()
