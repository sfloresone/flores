"""The flores-micro task catalog (Harbor task format)."""

from __future__ import annotations

import tomllib
from dataclasses import dataclass
from pathlib import Path

from flores_bench.paths import BENCH_ROOT

TASKS_DIR = BENCH_ROOT / "tasks" / "flores-micro"

REQUIRED_FILES = ("task.toml", "instruction.md", "environment/Dockerfile", "tests/test.sh", "solution/solve.sh")


@dataclass
class Task:
    name: str  # directory name, e.g. "fix-off-by-one"
    path: Path
    category: str
    difficulty: str
    why: str
    instruction: str


def load_task(path: Path) -> Task:
    missing = [f for f in REQUIRED_FILES if not (path / f).exists()]
    if missing:
        raise ValueError(f"{path.name}: missing {', '.join(missing)}")
    cfg = tomllib.loads((path / "task.toml").read_text())
    meta = cfg.get("metadata", {})
    for key in ("category", "difficulty", "why"):
        if not meta.get(key):
            raise ValueError(f"{path.name}: task.toml [metadata] needs '{key}'")
    expected = f"flores-micro/{path.name}"
    if cfg.get("task", {}).get("name") != expected:
        raise ValueError(f"{path.name}: [task] name must be {expected!r}")
    return Task(
        name=path.name,
        path=path,
        category=meta["category"],
        difficulty=meta["difficulty"],
        why=meta["why"],
        instruction=(path / "instruction.md").read_text().strip(),
    )


def load_tasks(selected: list[str] | None = None, root: Path = TASKS_DIR) -> list[Task]:
    tasks = [load_task(p) for p in sorted(root.iterdir()) if p.is_dir()]
    if selected:
        unknown = set(selected) - {t.name for t in tasks}
        if unknown:
            raise SystemExit(f"unknown task(s): {', '.join(sorted(unknown))}")
        tasks = [t for t in tasks if t.name in selected]
    return tasks


CATEGORY_TITLES = {
    "debug": "Depurar",
    "edit": "Editar",
    "shell": "Shell",
    "create": "Crear",
    "navigation": "Navegar el repo",
    "constraints": "Restricciones",
    "git": "Git",
}


def render_docs(tasks: list[Task]) -> str:
    lines = [
        "# flores-micro: tareas de la capa C",
        "",
        "> Generado con `uv run --group tasks flores-bench tasks list --write-docs` a partir de",
        "> `tasks/flores-micro/*/task.toml`. No editar a mano.",
        "",
        "Tareas pequeñas en Docker, en el formato de [Harbor](https://docs.harborframework.com/tasks/overview)",
        "(el runner de Terminal-Bench 2.0). Cada una aísla **una capacidad del harness**, no de",
        "inteligencia del modelo. Un modelo barato debería poder resolverlas todas, así que",
        "cuando una falla suele ser por cómo el harness gestiona las tools, el contexto o los errores.",
        "",
        "Cada tarea tiene `instruction.md` (lo único que ve el agente), `environment/` (la imagen),",
        "`tests/test.sh` (escribe la nota en `/logs/verifier/reward.txt`) y `solution/solve.sh`",
        "(la solución de referencia). `flores-bench tasks validate` comprueba que `oracle`",
        "(ejecuta la solución) saca 1 y `nop` (no hace nada) saca 0.",
        "",
        "| Tarea | Categoría | Dificultad | Qué capacidad mide |",
        "|---|---|---|---|",
    ]
    order = list(CATEGORY_TITLES)
    for t in sorted(tasks, key=lambda t: (order.index(t.category) if t.category in order else 99, t.name)):
        lines.append(f"| `{t.name}` | {CATEGORY_TITLES.get(t.category, t.category)} | {t.difficulty} | {t.why} |")
    lines += ["", "## Enunciados", ""]
    for t in sorted(tasks, key=lambda t: t.name):
        quoted = "\n".join(f"> {line}" if line else ">" for line in t.instruction.splitlines())
        lines += [f"### `{t.name}`", "", quoted, ""]
    return "\n".join(lines).rstrip() + "\n"
