"""Generate the flores-micro task directories (Harbor task format).

Run from benchmarks/:  uv run python tasks/generate.py

Tasks are plain files committed to git; this script only exists so the large or
repetitive inputs (a 3,000-line module, 1,500 tests, a 300k-line log) are
deterministic and reviewable as code instead of as blobs. Re-running it
rewrites every generated task; fix-off-by-one is hand-written and untouched.

Every task must pass `flores-bench tasks validate`: Harbor's `oracle` agent
(solution/solve.sh) scores 1 and `nop` scores 0.
"""

from __future__ import annotations

import hashlib
import random
import re
import shutil
import textwrap
from pathlib import Path

ROOT = Path(__file__).resolve().parent / "flores-micro"
PY = "python:3.13-slim"
# shipping_cost and its body, up to the next top-level def (large-file-edit).
SHIP_RE = r"def shipping_cost\(.*?(?=\ndef |\Z)"

TEST_SH_PY = """#!/bin/bash
# Reward 1 only if /tests/check.py passes.
mkdir -p /logs/verifier
if python3 /tests/check.py; then
  echo 1 > /logs/verifier/reward.txt
else
  echo 0 > /logs/verifier/reward.txt
fi
"""


def dedent(s: str) -> str:
    return textwrap.dedent(s).lstrip("\n")


def sha(text: str) -> str:
    return hashlib.sha256(text.encode()).hexdigest()


def write_task(
    name: str,
    *,
    category: str,
    difficulty: str,
    why: str,
    instruction: str,
    files: dict[str, str],
    solve: str,
    check: str | None = None,
    test_sh: str | None = None,
    image: str = PY,
    dockerfile_extra: str = "",
    tests_files: dict[str, str] | None = None,
    solution_files: dict[str, str] | None = None,
    agent_timeout: int = 600,
) -> None:
    d = ROOT / name
    if d.exists():
        shutil.rmtree(d)
    (d / "environment" / "app").mkdir(parents=True)
    (d / "tests").mkdir()
    (d / "solution").mkdir()
    (d / "task.toml").write_text(
        dedent(f"""
        schema_version = "1.4"

        [task]
        name = "flores-micro/{name}"

        [metadata]
        category = "{category}"
        difficulty = "{difficulty}"
        why = "{why}"

        [agent]
        timeout_sec = {agent_timeout}.0

        [verifier]
        timeout_sec = 120.0

        [environment]
        cpus = 1
        memory_mb = 2048
        """)
    )
    (d / "instruction.md").write_text(dedent(instruction).strip() + "\n")
    (d / "environment" / "Dockerfile").write_text(f"FROM {image}\nWORKDIR /app\n{dockerfile_extra}COPY app/ /app/\n")
    for rel, content in files.items():
        p = d / "environment" / "app" / rel
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_text(content)
    (d / "solution" / "solve.sh").write_text("#!/bin/bash\nset -euo pipefail\n" + dedent(solve))
    (d / "tests" / "test.sh").write_text(test_sh or TEST_SH_PY)
    if check is not None:
        (d / "tests" / "check.py").write_text(dedent(check))
    for rel, content in (tests_files or {}).items():
        (d / "tests" / rel).write_text(content)
    # Harbor's oracle uploads solution/ to /solution, so solve.sh can copy these.
    for rel, content in (solution_files or {}).items():
        (d / "solution" / rel).write_text(content)
    for f in (d / "solution" / "solve.sh", d / "tests" / "test.sh"):
        f.chmod(0o755)


# --------------------------------------------------------------------- tasks


def large_file_edit() -> None:
    rng = random.Random(1)
    parts = ['"""Rate tables. Generated; edit with care."""\n\n']
    target = 2400
    i = 0
    while sum(p.count("\n") for p in parts) < 3000:
        i += 1
        line_no = sum(p.count("\n") for p in parts)
        if (
            line_no >= target
            and "shipping_cost" not in "".join(parts[-3:])
            and i > 600
            and "def shipping" not in "".join(parts)
        ):
            parts.append(
                "def shipping_cost(weight_kg):\n"
                '    """Shipping cost in euros for a parcel of `weight_kg`."""\n'
                "    return weight_kg * 5\n\n\n"
            )
        f = round(rng.uniform(0.5, 2.0), 4)
        parts.append(f"def rate_{i:04d}(x):\n    return x * {f}\n\n\n")
    original = "".join(parts)
    write_task(
        "large-file-edit",
        category="edit",
        difficulty="easy",
        why="Editar una función en un fichero de 3.000 líneas sin reescribirlo ni truncarlo.",
        instruction="""
        In /app/rates.py, change `shipping_cost` so that it charges 4.5 per kg with a
        minimum charge of 10. Do not change anything else in the file.
        """,
        files={"rates.py": original},
        solve="""
        python3 - <<'EOF'
        p = "/app/rates.py"
        s = open(p).read()
        s = s.replace("    return weight_kg * 5\\n", "    return max(10, weight_kg * 4.5)\\n")
        open(p, "w").write(s)
        EOF
        """,
        check=f"""
        import hashlib, re, sys
        sys.path.insert(0, "/app")
        text = open("/app/rates.py").read()
        import rates
        ok = rates.shipping_cost(1) == 10 and rates.shipping_cost(4) == 18 and rates.shipping_cost(10) == 45
        # Everything outside shipping_cost must be byte-identical.
        rest = re.sub({SHIP_RE!r}, "", text, flags=re.S)
        same = hashlib.sha256(rest.encode()).hexdigest() == "{sha(re.sub(SHIP_RE, "", original, flags=re.S))}"
        print("behaviour ok:", ok, "rest unchanged:", same)
        sys.exit(0 if ok and same else 1)
        """,
    )


def rename_symbol() -> None:
    files = {
        "shop/__init__.py": '"""Tiny shop package."""\n\nfrom shop.cart import calc_total\n\n__all__ = ["calc_total"]\n',
        "shop/cart.py": dedent('''
            def calc_total(items, tax=0.21):
                """Return the total price of `items` (list of (price, qty)) including tax.

                See also: invoice.render, which relies on calc_total for the grand total.
                """
                net = sum(price * qty for price, qty in items)
                return round(net * (1 + tax), 2)
            '''),
        "shop/invoice.py": dedent("""
            from shop.cart import calc_total


            def render(items):
                return f"TOTAL {calc_total(items):.2f}"
            """),
        "shop/report.py": dedent("""
            from shop import cart


            def daily(orders):
                # each order is a list of (price, qty); calc_total adds VAT
                return sum(cart.calc_total(o) for o in orders)
            """),
        "tests/__init__.py": "",
        "tests/test_shop.py": dedent("""
            import unittest

            from shop import calc_total
            from shop.invoice import render
            from shop.report import daily


            class T(unittest.TestCase):
                def test_total(self):
                    self.assertEqual(calc_total([(10, 2)]), 24.2)

                def test_invoice(self):
                    self.assertEqual(render([(10, 1)]), "TOTAL 12.10")

                def test_daily(self):
                    self.assertEqual(daily([[(10, 1)], [(10, 1)]]), 24.2)
            """),
    }
    write_task(
        "rename-symbol",
        category="edit",
        difficulty="easy",
        why="Renombrar un símbolo en varios ficheros (definición, imports, llamadas, docs y tests) sin dejar ninguno.",
        instruction="""
        Rename the function `calc_total` to `compute_total` everywhere in /app:
        definition, imports, calls, `__all__`, comments, docstrings and tests.
        Behaviour must not change and `python -m unittest discover -s tests -t .` must pass.
        """,
        files=files,
        solve="""
        cd /app && grep -rl calc_total . | xargs sed -i 's/calc_total/compute_total/g'
        """,
        check="""
        import pathlib, subprocess, sys
        left = [str(p) for p in pathlib.Path("/app").rglob("*.py") if "calc_total" in p.read_text()]
        r = subprocess.run([sys.executable, "-m", "unittest", "discover", "-s", "tests", "-t", "."], cwd="/app")
        sys.path.insert(0, "/app")
        from shop import compute_total
        from shop.cart import compute_total as c2
        ok = not left and r.returncode == 0 and compute_total([(1, 1)]) == 1.21 and c2 is compute_total
        print("leftovers:", left)
        sys.exit(0 if ok else 1)
        """,
    )


def go_test() -> None:
    files = {
        "go.mod": "module example.com/dur\n\ngo 1.25\n",
        "dur.go": dedent("""
            // Package dur parses compact durations like "1h30m" or "45s" into seconds.
            package dur

            import (
            	"fmt"
            	"strconv"
            )

            // Seconds returns the number of seconds in s.
            func Seconds(s string) (int, error) {
            	total, num := 0, ""
            	for _, r := range s {
            		if r >= '0' && r <= '9' {
            			num += string(r)
            			continue
            		}
            		n, err := strconv.Atoi(num)
            		if err != nil {
            			return 0, fmt.Errorf("bad duration %q", s)
            		}
            		switch r {
            		case 'h':
            			total += n * 3600
            		case 'm':
            			total += n * 360
            		case 's':
            			total += n
            		default:
            			return 0, fmt.Errorf("bad unit %q", r)
            		}
            		num = ""
            	}
            	if num != "" {
            		return 0, fmt.Errorf("missing unit in %q", s)
            	}
            	return total, nil
            }
            """),
        "dur_test.go": dedent("""
            package dur

            import "testing"

            func TestSeconds(t *testing.T) {
            	cases := map[string]int{"45s": 45, "2m": 120, "1h30m": 5400, "1h1m1s": 3661}
            	for in, want := range cases {
            		got, err := Seconds(in)
            		if err != nil || got != want {
            			t.Errorf("Seconds(%q) = %d, %v; want %d", in, got, err, want)
            		}
            	}
            	if _, err := Seconds("10"); err == nil {
            		t.Error("missing unit must be an error")
            	}
            }
            """),
    }
    test_hash = sha(files["dur_test.go"])
    write_task(
        "fix-go-test",
        category="debug",
        difficulty="easy",
        why="Mismo bucle de depuración en Go: el harness debe usar la toolchain del proyecto, no solo Python.",
        instruction="""
        `go test ./...` fails in /app. Find the bug and fix it. Do not modify the tests.
        """,
        image="golang:1.25-bookworm",
        files=files,
        solve="""
        sed -i 's/n \\* 360$/n * 60/' /app/dur.go
        """,
        test_sh=dedent(f"""
            #!/bin/bash
            mkdir -p /logs/verifier
            cd /app
            if echo "{test_hash}  dur_test.go" | sha256sum -c --quiet && go test ./...; then
              echo 1 > /logs/verifier/reward.txt
            else
              echo 0 > /logs/verifier/reward.txt
            fi
            """),
    )


def js_test() -> None:
    files = {
        "package.json": '{\n  "name": "money",\n  "type": "module",\n  "scripts": { "test": "node --test" }\n}\n',
        "src/money.js": dedent("""
            // Format an amount of cents as a string with two decimals: 1999 -> "19.99".
            export function formatCents(cents) {
              const sign = cents < 0 ? "-" : "";
              const abs = Math.abs(cents);
              const euros = Math.floor(abs / 100);
              const rest = abs % 100;
              return `${sign}${euros}.${rest}`;
            }
            """),
        "test/money.test.js": dedent("""
            import { test } from "node:test";
            import assert from "node:assert/strict";
            import { formatCents } from "../src/money.js";

            test("formats cents", () => {
              assert.equal(formatCents(1999), "19.99");
              assert.equal(formatCents(1905), "19.05");
              assert.equal(formatCents(7), "0.07");
              assert.equal(formatCents(-250), "-2.50");
            });
            """),
    }
    test_hash = sha(files["test/money.test.js"])
    write_task(
        "fix-js-test",
        category="debug",
        difficulty="easy",
        why="Depurar en JavaScript con el runner nativo de Node: otra toolchain, mismo bucle.",
        instruction="""
        `npm test` fails in /app. Find the bug and fix it. Do not modify the tests.
        """,
        image="node:24-slim",
        files=files,
        solve="""
        sed -i 's/\\${rest}`/${String(rest).padStart(2, "0")}`/' /app/src/money.js
        """,
        test_sh=dedent(f"""
            #!/bin/bash
            mkdir -p /logs/verifier
            cd /app
            if echo "{test_hash}  test/money.test.js" | sha256sum -c --quiet && npm test; then
              echo 1 > /logs/verifier/reward.txt
            else
              echo 0 > /logs/verifier/reward.txt
            fi
            """),
    )


def log_needle() -> None:
    gen = dedent('''
        """Generate a large, deterministic access log (~300k lines)."""
        import random

        rng = random.Random(42)
        statuses = [200] * 80 + [404] * 10 + [500] * 5 + [503] * 5
        with open("/app/logs/access.log", "w") as f:
            for i in range(300_000):
                user = f"u-{rng.randint(1000, 9999)}"
                status = rng.choice(statuses)
                if user == "u-4242" and status == 503:
                    status = 200  # keep the needle unique
                rid = f"req-{rng.getrandbits(48):012x}"
                f.write(f"2026-11-05T10:{i // 6000 % 60:02d}:{i % 60:02d}Z GET /api/items/{i % 977} "
                        f"status={status} user={user} request_id={rid} ms={rng.randint(1, 900)}\\n")
                if i == 187_311:
                    f.write("2026-11-05T10:31:11Z POST /api/checkout status=503 user=u-4242 "
                            "request_id=req-7f3a9c0d1e2b ms=30001\\n")
        ''')
    write_task(
        "log-needle",
        category="shell",
        difficulty="easy",
        why="Buscar en un log de ~30 MB: el agente debe filtrar con la shell, no volcar el fichero al contexto.",
        instruction="""
        /app/logs/access.log is a large access log. Find the request_id of the only
        request from user u-4242 that returned HTTP 503, and write just that id
        (for example `req-0123456789ab`) to /app/answer.txt.
        """,
        files={"gen_log.py": gen},
        dockerfile_extra="COPY app/gen_log.py /tmp/gen_log.py\nRUN mkdir -p /app/logs && python3 /tmp/gen_log.py && rm /tmp/gen_log.py\n",
        solve="""
        grep 'user=u-4242 ' /app/logs/access.log | grep 'status=503' | sed -E 's/.*request_id=([^ ]+).*/\\1/' > /app/answer.txt
        """,
        check="""
        import sys
        try:
            got = open("/app/answer.txt").read().strip()
        except FileNotFoundError:
            sys.exit(1)
        sys.exit(0 if got == "req-7f3a9c0d1e2b" else 1)
        """,
    )


def csv_report() -> None:
    rng = random.Random(7)
    files = {}
    for region in ("north", "south", "east", "west", "central"):
        rows = ["order_id,region,amount,status"]
        for i in range(40):
            status = rng.choice(["paid"] * 6 + ["refunded", "pending"])
            rows.append(f"{region[:1].upper()}{i:03d},{region},{rng.randint(100, 99999) / 100:.2f},{status}")
        files[f"data/{region}.csv"] = "\n".join(rows) + "\n"
    write_task(
        "csv-report",
        category="shell",
        difficulty="medium",
        why="Encadenar lectura de varios ficheros y producir una salida con formato exacto.",
        instruction="""
        The CSV files in /app/data contain orders. Write /app/report.json with this shape:

            {"total": <sum of amount>, "by_region": {"<region>": <sum of amount>, ...}}

        Only count rows whose status is `paid`. Round every number to 2 decimals.
        """,
        files=files,
        solve="""
        python3 - <<'EOF'
        import csv, glob, json
        by = {}
        for f in glob.glob("/app/data/*.csv"):
            for r in csv.DictReader(open(f)):
                if r["status"] == "paid":
                    by[r["region"]] = by.get(r["region"], 0) + float(r["amount"])
        by = {k: round(v, 2) for k, v in by.items()}
        json.dump({"total": round(sum(by.values()), 2), "by_region": by}, open("/app/report.json", "w"))
        EOF
        """,
        check="""
        import csv, glob, json, sys
        by = {}
        for f in glob.glob("/app/data/*.csv"):
            for r in csv.DictReader(open(f)):
                if r["status"] == "paid":
                    by[r["region"]] = by.get(r["region"], 0) + float(r["amount"])
        try:
            got = json.load(open("/app/report.json"))
        except Exception as e:
            print(e); sys.exit(1)
        close = lambda a, b: abs(float(a) - b) < 0.011
        ok = set(got.get("by_region", {})) == set(by)
        ok = ok and all(close(got["by_region"][k], v) for k, v in by.items())
        ok = ok and close(got.get("total", -1), sum(by.values()))
        sys.exit(0 if ok else 1)
        """,
    )


def dont_touch_vendor() -> None:
    vendor = dedent('''
        # Third-party code (semver-lite 0.3). DO NOT MODIFY: synced from upstream.


        def bump(version, part):
            """Return `version` with `part` ("major", "minor" or "patch") incremented."""
            major, minor, patch = (int(x) for x in version.split("."))
            if part == "major":
                return f"{major + 1}.0.0"
            if part == "minor":
                return f"{major}.{minor + 1}.0"
            if part == "patch":
                return f"{major}.{minor}.{patch + 1}"
            raise ValueError(part)
        ''')
    files = {
        "vendor/__init__.py": "",
        "vendor/semver_lite.py": vendor,
        "release.py": dedent("""
            from vendor.semver_lite import bump


            def next_release(current, breaking=False, feature=False):
                if breaking:
                    return bump("major", current)
                if feature:
                    return bump("minor", current)
                return bump("patch", current)
            """),
        "test_release.py": dedent("""
            import unittest

            from release import next_release


            class T(unittest.TestCase):
                def test_patch(self):
                    self.assertEqual(next_release("1.2.3"), "1.2.4")

                def test_feature(self):
                    self.assertEqual(next_release("1.2.3", feature=True), "1.3.0")

                def test_breaking(self):
                    self.assertEqual(next_release("1.2.3", breaking=True), "2.0.0")
            """),
    }
    write_task(
        "dont-touch-vendor",
        category="constraints",
        difficulty="easy",
        why="Respetar una restricción explícita: el error aparece en vendor/, pero el bug está en el código propio.",
        instruction="""
        Make `python -m unittest` pass in /app. Files under /app/vendor are third-party
        code and must not be modified.
        """,
        files=files,
        solve="""
        sed -i -E 's/bump\\("(major|minor|patch)", current\\)/bump(current, "\\1")/' /app/release.py
        """,
        check=f"""
        import hashlib, subprocess, sys
        v = hashlib.sha256(open("/app/vendor/semver_lite.py", "rb").read()).hexdigest()
        t = hashlib.sha256(open("/app/test_release.py", "rb").read()).hexdigest()
        r = subprocess.run([sys.executable, "-m", "unittest"], cwd="/app")
        ok = v == "{sha(vendor)}" and t == "{sha(files["test_release.py"])}" and r.returncode == 0
        sys.exit(0 if ok else 1)
        """,
    )


def implement_from_spec() -> None:
    stub = dedent('''
        def slugify(text, max_length=50):
            """Turn `text` into a URL slug.

            Rules:
            - lowercase;
            - accented Latin letters become their plain ASCII letter (á -> a, ñ -> n, ç -> c);
            - every run of characters that are not a-z or 0-9 becomes a single "-";
            - no leading or trailing "-";
            - if the slug is longer than `max_length`, cut it at the last "-" that keeps
              it within `max_length` (never cut a word in half). If a single word is longer
              than `max_length`, cut that word at `max_length`.
            """
            raise NotImplementedError
        ''')
    solution = dedent('''
        import re
        import unicodedata


        def slugify(text, max_length=50):
            """See the task description."""
            ascii_text = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode()
            slug = re.sub(r"[^a-z0-9]+", "-", ascii_text.lower()).strip("-")
            if len(slug) <= max_length:
                return slug
            cut = slug[: max_length + 1]
            if "-" in cut:
                return cut[: cut.rindex("-")].strip("-")
            return slug[:max_length]
        ''')
    write_task(
        "implement-from-spec",
        category="create",
        difficulty="medium",
        why="Implementar desde una especificación con tests ocultos: generalizar, no ajustarse a los tests visibles.",
        instruction="""
        Implement `slugify` in /app/slug.py following its docstring exactly. The examples
        in /app/test_slug.py pass only with a correct implementation, but they are not
        the full test suite.
        """,
        files={
            "slug.py": stub,
            "test_slug.py": dedent("""
                import unittest

                from slug import slugify


                class T(unittest.TestCase):
                    def test_examples(self):
                        self.assertEqual(slugify("Hello, World!"), "hello-world")
                        self.assertEqual(slugify("  Árbol de Navidad  "), "arbol-de-navidad")
                """),
        },
        solve="""
        cp /solution/slug.py /app/slug.py
        """,
        solution_files={"slug.py": solution},
        check="""
        import sys
        sys.path.insert(0, "/app")
        from slug import slugify
        cases = [
            (("Hello, World!",), "hello-world"),
            (("  Árbol de Navidad  ",), "arbol-de-navidad"),
            (("Ça va? Ñandú!!",), "ca-va-nandu"),
            (("---a---b---",), "a-b"),
            (("C++ & Go: 2026 edition",), "c-go-2026-edition"),
            (("one two three four", 9), "one-two"),
            (("one two three four", 13), "one-two-three"),
            (("supercalifragilistic", 5), "super"),
            (("", ), ""),
            (("Zoë's café", 50), "zoe-s-cafe"),
        ]
        bad = []
        for args, want in cases:
            try:
                got = slugify(*args)
            except Exception as e:
                got = repr(e)
            if got != want:
                bad.append((args, want, got))
        print(bad)
        sys.exit(0 if not bad else 1)
        """,
    )


def config_from_readme() -> None:
    files = {
        "README.md": dedent("""
            # inventory-service

            ## Configuration

            Copy `config/settings.example.toml` to `config/settings.toml` and edit it:

            - `environment` must be `"production"` for deployed instances.
            - `port` must be between 1024 and 65535.
            - `database_url` uses the `sqlite:///` scheme; production uses `sqlite:///data/inventory.db`.

            Check the configuration with `python main.py --check`.
            """),
        "config/settings.example.toml": dedent("""
            environment = "development"
            port = 8000
            database_url = "sqlite:///:memory:"
            """),
        "main.py": dedent("""
            import sys
            import tomllib
            from pathlib import Path

            CONFIG = Path(__file__).parent / "config" / "settings.toml"


            def check():
                if not CONFIG.exists():
                    sys.exit(f"missing {CONFIG}; see README.md")
                cfg = tomllib.loads(CONFIG.read_text())
                assert cfg["environment"] in ("development", "production"), "bad environment"
                assert 1024 <= int(cfg["port"]) <= 65535, "bad port"
                assert cfg["database_url"].startswith("sqlite:///"), "bad database_url"
                print("config ok:", cfg["environment"])


            if __name__ == "__main__" and "--check" in sys.argv:
                check()
            """),
    }
    write_task(
        "config-from-readme",
        category="navigation",
        difficulty="easy",
        why="Leer la documentación del propio repo para configurar algo correctamente, no adivinar.",
        instruction="""
        Configure the service in /app for a production deployment, following the
        project's README. `python main.py --check` must succeed.
        """,
        files=files,
        solve="""
        cat > /app/config/settings.toml <<'EOF'
        environment = "production"
        port = 8000
        database_url = "sqlite:///data/inventory.db"
        EOF
        """,
        check="""
        import subprocess, sys, tomllib
        r = subprocess.run([sys.executable, "main.py", "--check"], cwd="/app")
        try:
            cfg = tomllib.load(open("/app/config/settings.toml", "rb"))
        except Exception:
            sys.exit(1)
        ok = r.returncode == 0 and cfg.get("environment") == "production"
        ok = ok and cfg.get("database_url") == "sqlite:///data/inventory.db"
        sys.exit(0 if ok else 1)
        """,
    )


def git_commit() -> None:
    files = {
        "src/inbox.py": "def recieve(msg):\n    # recieve a message and store it\n    return msg\n",
        "src/notes.md": "We recieve about 30 messages per hour.\n",
        "CHANGELOG.md": "- 0.1: first release (we recieve messages).\n",
    }
    write_task(
        "git-commit",
        category="git",
        difficulty="easy",
        why="Usar git como lo haría una persona: cambio acotado, un commit con el formato pedido y el árbol limpio.",
        instruction="""
        /app is a git repository. Fix the typo `recieve` -> `receive` in every file under
        /app/src (and only there: CHANGELOG.md is historical and must stay as is).
        Commit the change as a single new commit whose message starts with `fix: `.
        Leave the working tree clean.
        """,
        files=files,
        dockerfile_extra=(
            "RUN apt-get update && apt-get install -y --no-install-recommends git && rm -rf /var/lib/apt/lists/*\n"
        ),
        # The repo is created after COPY, so the Dockerfile gets an extra RUN below.
        solve="""
        cd /app
        sed -i 's/recieve/receive/g' src/*
        git add -A src
        git commit -q -m "fix: typo recieve -> receive"
        """,
        check="""
        import subprocess, sys
        def git(*a):
            return subprocess.run(["git", "-C", "/app", *a], capture_output=True, text=True).stdout
        msgs = git("log", "--format=%s").splitlines()
        tree = git("grep", "-n", "recieve", "HEAD", "--", "src")
        changelog_ok = "recieve" in git("show", "HEAD:CHANGELOG.md")
        clean = git("status", "--porcelain").strip() == ""
        ok = len(msgs) == 2 and msgs[0].startswith("fix: ") and not tree and changelog_ok and clean
        print(msgs, repr(tree), changelog_ok, clean)
        sys.exit(0 if ok else 1)
        """,
    )
    df = ROOT / "git-commit" / "environment" / "Dockerfile"
    df.write_text(
        df.read_text() + 'RUN git config --global user.email "dev@example.com" && git config --global user.name "dev" '
        "&& git config --global --add safe.directory /app "
        '&& git init -q && git add -A && git commit -q -m "initial import"\n'
    )


def yaml_edit() -> None:
    original = dedent("""
        # Deployment for the shop stack.
        # Owner: platform team. Reviewed 2026-10.
        services:
          api:
            image: shop/api:1.8.2
            # scaled for the autumn campaign
            replicas: 2
            env:
              PORT: "8080"
          worker:
            image: shop/worker:1.8.2
            replicas: 1  # one is enough, jobs are idempotent
            env:
              QUEUE: jobs
        # end of file
        """)
    write_task(
        "yaml-edit",
        category="edit",
        difficulty="easy",
        why="Edición precisa de configuración: cambiar dos valores sin perder comentarios ni el orden.",
        instruction="""
        In /app/deploy.yaml set the `replicas` of the `api` service to 5, and add the
        environment variable `LOG_LEVEL` with value `debug` to the `worker` service.
        Keep every comment and the order of the keys.
        """,
        files={"deploy.yaml": original},
        solve="""
        python3 - <<'EOF'
        p = "/app/deploy.yaml"
        s = open(p).read()
        s = s.replace("    replicas: 2\\n", "    replicas: 5\\n")
        s = s.replace("      QUEUE: jobs\\n", "      QUEUE: jobs\\n      LOG_LEVEL: debug\\n")
        open(p, "w").write(s)
        EOF
        """,
        check=f"""
        import sys
        original = {original!r}
        got = open("/app/deploy.yaml").read()
        norm = lambda s: [l.rstrip().replace('"', "").replace("'", "") for l in s.splitlines() if l.strip()]
        g = norm(got)
        # every original line except api replicas must still be there, in order
        keep = [l for l in norm(original) if l.strip() != "replicas: 2"]
        it = iter(g)
        in_order = all(any(x == want for x in it) for want in keep)
        api = g[g.index("  api:"): g.index("  worker:")]
        worker = g[g.index("  worker:"):]
        ok = in_order and "    replicas: 5" in api and any(l.strip() == "LOG_LEVEL: debug" for l in worker)
        ok = ok and not any("LOG_LEVEL" in l for l in api)
        print(in_order, api, worker)
        sys.exit(0 if ok else 1)
        """,
    )


def hanging_test() -> None:
    files = {
        "service.py": dedent('''
            import time


            class Service:
                def __init__(self, boot_steps=3):
                    self.remaining = boot_steps

                def step(self):
                    if self.remaining > 0:
                        self.remaining -= 1

                @property
                def ready(self):
                    return self.remaining == 0


            def wait_until_ready(service, poll=0.01):
                """Advance the service until it is ready and return how many polls it took."""
                polls = 0
                while not service.ready:
                    polls += 1
                    time.sleep(poll)
                return polls
            '''),
        "test_service.py": dedent("""
            import unittest

            from service import Service, wait_until_ready


            class T(unittest.TestCase):
                def test_ready(self):
                    self.assertEqual(wait_until_ready(Service(3)), 3)

                def test_already_ready(self):
                    self.assertEqual(wait_until_ready(Service(0)), 0)
            """),
    }
    write_task(
        "hanging-test",
        category="debug",
        difficulty="medium",
        why="El test se cuelga para siempre: el harness tiene que cortar comandos con timeout y seguir trabajando.",
        instruction="""
        `python -m unittest` in /app never finishes. Find out why and fix the bug so the
        suite passes quickly. Do not modify the tests.
        """,
        files=files,
        solve="""
        sed -i 's/        polls += 1/        service.step()\\n        polls += 1/' /app/service.py
        """,
        check=f"""
        import hashlib, subprocess, sys
        t = hashlib.sha256(open("/app/test_service.py", "rb").read()).hexdigest()
        try:
            r = subprocess.run([sys.executable, "-m", "unittest"], cwd="/app", timeout=30)
        except subprocess.TimeoutExpired:
            sys.exit(1)
        sys.exit(0 if r.returncode == 0 and t == "{sha(files["test_service.py"])}" else 1)
        """,
    )


def noisy_tests() -> None:
    primes, n = [], 2
    while len(primes) < 1500:
        if all(n % p for p in primes if p * p <= n):
            primes.append(n)
        n += 1
    data = list(primes)
    data[1006] = data[1006] + 2  # the corrupted entry
    tests = dedent("""
        import unittest

        from primes import nth_prime, is_prime


        class TestPrimes(unittest.TestCase):
            pass


        def make(i):
            def test(self):
                p = nth_prime(i)
                for k in range(15):
                    print(f"[debug] checking candidate {i}:{k} value={p} window={p - k}..{p + k}")
                self.assertTrue(is_prime(p), f"nth_prime({i}) = {p} is not prime")
            return test


        for i in range(1500):
            setattr(TestPrimes, f"test_prime_{i:04d}", make(i))
        """)
    write_task(
        "noisy-tests",
        category="debug",
        difficulty="medium",
        why="1.500 tests y unas 22.000 líneas de salida con un único fallo: gestionar salidas enormes sin perder el error.",
        instruction="""
        One test in /app fails among many (`python -m unittest`; the output is long).
        Fix the underlying problem. Do not modify test_primes.py or primes.py.
        """,
        files={
            "primes.py": dedent("""
                from pathlib import Path

                TABLE = [int(x) for x in (Path(__file__).parent / "data" / "primes.txt").read_text().split()]


                def nth_prime(i):
                    return TABLE[i]


                def is_prime(n):
                    if n < 2:
                        return False
                    k = 2
                    while k * k <= n:
                        if n % k == 0:
                            return False
                        k += 1
                    return True
                """),
            "test_primes.py": tests,
            "data/primes.txt": "\n".join(map(str, data)) + "\n",
        },
        solve=f"""
        sed -i 's/^{data[1006]}$/{primes[1006]}/' /app/data/primes.txt
        """,
        check=f"""
        import hashlib, subprocess, sys
        h = lambda p: hashlib.sha256(open(p, "rb").read()).hexdigest()
        r = subprocess.run([sys.executable, "-m", "unittest", "-q"], cwd="/app", capture_output=True, timeout=120)
        lines = open("/app/data/primes.txt").read().split()
        ok = r.returncode == 0 and len(lines) == 1500 and lines[1006] == "{primes[1006]}"
        ok = ok and h("/app/test_primes.py") == "{sha(tests)}"
        sys.exit(0 if ok else 1)
        """,
    )


def add_parameter() -> None:
    files = {
        "notify.py": dedent('''
            SENT = []


            def send(user, message):
                """Queue a notification for `user`."""
                SENT.append({"user": user, "message": message})
            '''),
        "services/__init__.py": "",
        "services/billing.py": dedent("""
            from notify import send


            def invoice_ready(user, number):
                send(user, f"Invoice {number} is ready")


            def payment_failed(user):
                send(user, "Your payment failed")
            """),
        "services/alerts.py": dedent("""
            import notify


            def disk_full(user, host):
                notify.send(user, f"Disk full on {host}")


            def service_down(user, name):
                notify.send(user, message=f"{name} is down")
            """),
        "services/digest.py": dedent("""
            from notify import send as notify_user


            def weekly(user, items):
                # billing summary, goes by email like the rest of billing
                notify_user(user, f"{len(items)} invoices this week")
            """),
    }
    write_task(
        "add-parameter",
        category="edit",
        difficulty="medium",
        why="Cambiar una firma y actualizar todas las llamadas, incluidas las que usan alias o argumentos con nombre.",
        instruction="""
        Add a required keyword-only parameter `channel` to `notify.send` (stored in each
        SENT entry under the key "channel"). Then update every call site in
        /app/services: billing notifications (services/billing.py and services/digest.py)
        use channel="email", alerts (services/alerts.py) use channel="sms".
        """,
        files=files,
        solve="""
        cd /app
        python3 - <<'EOF'
        import re
        def sub(p, a, b):
            s = open(p).read(); s = s.replace(a, b); open(p, "w").write(s)
        sub("notify.py", "def send(user, message):", "def send(user, message, *, channel):")
        sub("notify.py", '"message": message}', '"message": message, "channel": channel}')
        for p, ch in (("services/billing.py", "email"), ("services/digest.py", "email"), ("services/alerts.py", "sms")):
            s = open(p).read()
            s = re.sub(r'((?:notify\\.send|send|notify_user)\\(user, [^\\n]*)\\)\\n', lambda m: m.group(1) + f', channel="{ch}")\\n', s)
            open(p, "w").write(s)
        EOF
        """,
        check="""
        import sys
        sys.path.insert(0, "/app")
        import notify
        from services import alerts, billing, digest
        try:
            notify.send("x", "y")
            required = False
        except TypeError:
            required = True
        try:
            notify.send("x", "y", "email")
            kwonly = False
        except TypeError:
            kwonly = True
        notify.SENT.clear()
        billing.invoice_ready("ana", 7); billing.payment_failed("ana"); digest.weekly("ana", [1, 2])
        alerts.disk_full("bob", "h1"); alerts.service_down("bob", "db")
        chans = [s.get("channel") for s in notify.SENT]
        ok = required and kwonly and chans == ["email", "email", "email", "sms", "sms"]
        print(required, kwonly, chans)
        sys.exit(0 if ok else 1)
        """,
    )


def find_todos() -> None:
    files = {
        "src/app.py": "import os\n\n# TODO: read config from env\nPORT = 8000\n\n\ndef main():\n    pass  # TODO handle signals\n",
        "src/db/models.py": "class User:\n    # TODO(ana): add email validation\n    pass\n",
        "src/db/__init__.py": "",
        "src/web/views.js": "export function home() {\n  // TODO: cache this\n  return 'home';\n}\n",
        "src/vendor/lib.py": "# TODO: upstream issue #12 (vendored, ignore)\n",
        "src/README.md": "Not code. TODO: this one does not count, only .py and .js files.\n",
    }
    write_task(
        "find-todos",
        category="shell",
        difficulty="easy",
        why="Búsqueda con criterios (extensiones, exclusiones) y salida con formato exacto.",
        instruction="""
        List every TODO comment in the .py and .js files under /app/src, excluding
        /app/src/vendor. Write them to /app/todos.txt, one per line, as

            <path relative to /app/src>:<line number>

        sorted alphabetically (for example `app.py:3`).
        """,
        files=files,
        solve="""
        cd /app/src && grep -rn --include='*.py' --include='*.js' 'TODO' . | grep -v '^./vendor/' | cut -d: -f1,2 | sed 's#^\\./##' | sort > /app/todos.txt
        """,
        check="""
        import sys
        want = ["app.py:3", "app.py:8", "db/models.py:2", "web/views.js:2"]
        try:
            got = [l.strip() for l in open("/app/todos.txt") if l.strip()]
        except FileNotFoundError:
            sys.exit(1)
        print(got)
        sys.exit(0 if got == want else 1)
        """,
    )


WCL = dedent("""
        #!/usr/bin/env python3
        import json, sys
        args = sys.argv[1:]
        as_json = "--json" in args
        paths = [a for a in args if a != "--json"]
        out, status, tot = {}, 0, [0, 0, 0]
        for p in paths:
            try:
                data = open(p, "rb").read()
            except FileNotFoundError:
                print(f"wcl: {p}: not found", file=sys.stderr)
                status = 1
                continue
            c = [data.count(b"\\n"), len(data.split()), len(data)]
            out[p] = c
            tot = [a + b for a, b in zip(tot, c)]
        if as_json:
            print(json.dumps({p: {"lines": c[0], "words": c[1], "bytes": c[2]} for p, c in out.items()}))
        else:
            for p, c in out.items():
                print(f"{c[0]} {c[1]} {c[2]} {p}")
            if len(out) > 1:
                print(f"{tot[0]} {tot[1]} {tot[2]} total")
        sys.exit(status)
        """)


def write_cli() -> None:
    write_task(
        "write-cli",
        category="create",
        difficulty="medium",
        why="Crear un programa nuevo a partir de una especificación de comportamiento (args, salida, errores).",
        instruction="""
        Create an executable `/app/bin/wcl` (any language available in the container)
        that counts lines, words and bytes of the files given as arguments:

        - `wcl FILE...` prints one line per file: `<lines> <words> <bytes> <path>`,
          then, if there is more than one file, a final line `<lines> <words> <bytes> total`.
        - `wcl --json FILE...` prints a single JSON object mapping each path to
          `{"lines": n, "words": n, "bytes": n}` (no total).
        - If a file does not exist, print `wcl: <path>: not found` to stderr, skip it,
          and exit with status 1 after processing the rest (otherwise exit 0).
        """,
        files={"README.md": "Put the tool in bin/wcl.\n"},
        solve="""
        mkdir -p /app/bin
        cp /solution/wcl /app/bin/wcl
        chmod +x /app/bin/wcl
        """,
        solution_files={"wcl": WCL},
        check="""
        import json, os, subprocess, sys, tempfile
        d = tempfile.mkdtemp()
        a, b = os.path.join(d, "a.txt"), os.path.join(d, "b.txt")
        open(a, "w").write("one two\\nthree\\n")
        open(b, "w").write("x y z")
        run = lambda *args: subprocess.run(["/app/bin/wcl", *args], capture_output=True, text=True)
        try:
            r1 = run(a, b); r2 = run("--json", a); r3 = run(a, os.path.join(d, "nope"))
        except OSError as e:
            print(e); sys.exit(1)
        ok = r1.returncode == 0 and r1.stdout.splitlines() == [f"2 3 14 {a}", f"0 3 5 {b}", "2 6 19 total"]
        ok = ok and r2.returncode == 0 and json.loads(r2.stdout) == {a: {"lines": 2, "words": 3, "bytes": 14}}
        ok = ok and r3.returncode == 1 and r3.stdout.strip() == f"2 3 14 {a}" and "not found" in r3.stderr
        print(r1.stdout, r2.stdout, r3.stdout, r3.stderr)
        sys.exit(0 if ok else 1)
        """,
    )


def multi_file_answer() -> None:
    files = {
        "config/default.toml": '[server]\nhost = "0.0.0.0"\nport = 8000\n',
        "config/production.toml": "[server]\nport = 8080\n",
        ".env": "APP_PORT=9090\n",
        "settings.py": dedent('''
            """Effective settings: default.toml, then <APP_ENV>.toml, then .env overrides."""
            import os
            import tomllib
            from pathlib import Path

            ROOT = Path(__file__).parent


            def load():
                env = os.environ.get("APP_ENV", "development")
                cfg = tomllib.loads((ROOT / "config" / "default.toml").read_text())
                layer = ROOT / "config" / f"{env}.toml"
                if layer.exists():
                    cfg["server"].update(tomllib.loads(layer.read_text()).get("server", {}))
                # .env is a developer convenience and is ignored in production
                if env != "production" and (ROOT / ".env").exists():
                    for line in (ROOT / ".env").read_text().splitlines():
                        key, _, value = line.partition("=")
                        if key == "APP_PORT":
                            cfg["server"]["port"] = int(value)
                return cfg
            '''),
    }
    write_task(
        "multi-file-answer",
        category="navigation",
        difficulty="easy",
        why="Responder una pregunta que exige leer varios ficheros y el código que los combina (con una trampa).",
        instruction="""
        Which TCP port will the app in /app listen on when it runs with APP_ENV=production?
        Write only the number to /app/answer.txt.
        """,
        files=files,
        solve="""
        cd /app && APP_ENV=production python3 -c 'import settings; print(settings.load()["server"]["port"])' > answer.txt
        """,
        check="""
        import sys
        try:
            got = open("/app/answer.txt").read().strip()
        except FileNotFoundError:
            sys.exit(1)
        sys.exit(0 if got == "8080" else 1)
        """,
    )


ALL = [
    large_file_edit,
    rename_symbol,
    go_test,
    js_test,
    log_needle,
    csv_report,
    dont_touch_vendor,
    implement_from_spec,
    config_from_readme,
    git_commit,
    yaml_edit,
    hanging_test,
    noisy_tests,
    add_parameter,
    find_todos,
    write_cli,
    multi_file_answer,
]

if __name__ == "__main__":
    for fn in ALL:
        fn()
    print(f"generated {len(ALL)} tasks in {ROOT}")
