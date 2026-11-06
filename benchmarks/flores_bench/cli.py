"""flores-bench command line.

flores-bench doctor                       what is installed, which versions
flores-bench build                        compile the Go helpers and flores (apps/cli)
flores-bench pinned [--agent NAME]       npm specs of the pinned competitor versions
flores-bench perf startup [opts]          layer A: start-up latency/RSS
flores-bench perf hello   [opts]          layer A: harness overhead per trivial turn
flores-bench harness      [opts]          layer B: behaviour under scripted failures
flores-bench scenarios [--write-docs]     list layer B scenarios / regenerate their docs
flores-bench tasks list                   layer C: flores-micro tasks
flores-bench tasks validate              layer C: oracle must score 1, nop 0 (Docker, free)
flores-bench tasks run [opts]            layer C: real models through the meter (costs money)
flores-bench check-budgets RESULT.json    PASS/FAIL against suites/perf/budgets.toml
flores-bench publish RESULT.json          copy into published/ + regenerate report
flores-bench report                       regenerate published/README.md
flores-bench trajectory validate FILE     check a trajectory/v0 JSONL file
"""

from __future__ import annotations

import argparse
import json
import shutil
import sys
from pathlib import Path

from flores_bench import budgets, report
from flores_bench.build import build_tools
from flores_bench.config import load_agents
from flores_bench.envinfo import machine_info, platform_key, tool_versions
from flores_bench.paths import RESULTS_DIR


def _save(result: dict, out: Path | None) -> Path:
    if out is None:
        stamp = result["created_at"].replace(":", "").replace("-", "")
        out = RESULTS_DIR / result["suite"] / f"{stamp}-{platform_key()}.json"
    out.parent.mkdir(parents=True, exist_ok=True)
    out.write_text(json.dumps(result, indent=2) + "\n")
    return out


def _print_summary(result: dict) -> None:
    print()
    print(report.render_table(report.compact(result)))
    print()


def cmd_doctor(args: argparse.Namespace) -> int:
    print("machine:", json.dumps(machine_info()))
    print("tools:  ", json.dumps(tool_versions()))
    print()
    print(f"{'agent':<14} {'installed':<10} {'version':<12} {'pinned':<10} model")
    for a in load_agents():
        ad = a.adapter
        v = ad.version() if ad.available() else None
        mark = "" if not (v and a.pinned_version and v != a.pinned_version) else "  ⚠ version differs"
        print(
            f"{ad.name:<14} {'yes' if ad.available() else 'no':<10} {v or '-':<12} "
            f"{a.pinned_version or '-':<10} {a.model}{mark}"
        )
    missing = [t for t in ("go", "hyperfine", "docker") if not shutil.which(t)]
    if missing:
        print("\nmissing tools:", ", ".join(missing))
    return 0


def cmd_pinned(args: argparse.Namespace) -> int:
    """Print `package@version` for every npm-installable agent (used by CI).

    With --agent, print only that agent's pinned version (e.g. for fx's installer).
    """
    if args.agent:
        a = load_agents([args.agent])[0]
        if not a.pinned_version:
            print(f"{args.agent} has no pinned version", file=sys.stderr)
            return 1
        print(a.pinned_version)
        return 0
    for a in load_agents():
        if a.npm and a.pinned_version:
            print(f"{a.npm}@{a.pinned_version}")
    return 0


def cmd_build(args: argparse.Namespace) -> int:
    build_tools()
    print("built benchmarks/.bin/{mockprovider,flores-stub,meter,measure,flores}")
    return 0


def cmd_perf(args: argparse.Namespace) -> int:
    from flores_bench.perf import hello, startup

    build_tools()
    agents = load_agents(args.agents.split(",") if args.agents else None)
    if args.quick:
        args.runs, args.warmup, args.rounds = max(1, args.runs // 5), 1, 1
    print(
        f"{args.suite}: {args.rounds} rounds × {args.runs} runs (+{args.warmup} warmup) "
        f"for {', '.join(a.adapter.name for a in agents)}"
    )
    if args.suite == "startup":
        result = startup.run(agents, args.runs, args.warmup, args.rounds, args.seed)
    else:
        result = hello.run(agents, args.runs, args.warmup, args.rounds, args.seed)
    path = _save(result, args.out)
    _print_summary(result)
    print(f"raw result: {path}")
    ok = budgets.check(result)
    if args.publish:
        print(f"published:  {report.publish(result)}")
    return 0 if ok or not args.enforce else 1


def cmd_harness(args: argparse.Namespace) -> int:
    from flores_bench.harness import runner, scenarios

    build_tools()
    agents = load_agents(args.agents.split(",") if args.agents else None)
    selected = scenarios.load_all()
    if args.scenarios:
        wanted = set(args.scenarios.split(","))
        unknown = wanted - {s.name for s in selected}
        if unknown:
            raise SystemExit(f"unknown scenarios: {', '.join(sorted(unknown))}")
        selected = [s for s in selected if s.name in wanted]
    print(f"harness: {len(selected)} scenarios × {args.runs} runs for {', '.join(a.adapter.name for a in agents)}")
    result = runner.run(agents, selected, runs=args.runs)
    path = _save(result, args.out)
    print()
    print(report.render_harness(result))
    print(f"\nraw result: {path}")
    ok = budgets.check_harness(result)
    if args.publish:
        print(f"published:  {report.publish(result)}")
    return 0 if ok or not args.enforce else 1


def cmd_scenarios(args: argparse.Namespace) -> int:
    from flores_bench.harness import scenarios
    from flores_bench.paths import BENCH_ROOT

    all_ = scenarios.load_all()
    if args.write_docs:
        out = BENCH_ROOT / "docs" / "harness-scenarios.md"
        out.write_text(scenarios.render_docs(all_))
        print(out)
        return 0
    for s in all_:
        print(f"{s.category:<16} {s.name:<28} {s.title}")
    return 0


def cmd_check_budgets(args: argparse.Namespace) -> int:
    ok = True
    for f in args.results:
        result = json.loads(Path(f).read_text())
        print(f"{result['suite']} ({result['machine']['platform']}): {f}")
        if result["suite"] == "harness":
            ok &= budgets.check_harness(result)
        else:
            ok &= budgets.check(result, enforce=True if args.force else None)
    return 0 if ok else 1


def cmd_tasks(args: argparse.Namespace) -> int:
    from flores_bench.paths import RESULTS_DIR as RD
    from flores_bench.tasks import catalog, runner

    tasks = catalog.load_tasks(args.tasks.split(",") if getattr(args, "tasks", None) else None)
    if args.action == "list":
        if args.write_docs:
            from flores_bench.paths import BENCH_ROOT

            out = BENCH_ROOT / "docs" / "flores-micro.md"
            out.write_text(catalog.render_docs(tasks))
            print(out)
            return 0
        for t in tasks:
            print(f"{t.category:<10} {t.difficulty:<7} {t.name:<28} {t.why}")
        return 0
    build_tools()
    jobs = (RD / "jobs").resolve()
    jobs.mkdir(parents=True, exist_ok=True)
    if args.action == "validate":
        res = runner.validate(tasks, jobs, concurrent=args.concurrent)
        bad = 0
        for name, r in res.items():
            mark = "PASS" if r["ok"] else "FAIL"
            bad += not r["ok"]
            err = r.get("oracle_error") or r.get("nop_error") or ""
            print(f"  {mark}  {name:<28} oracle={r.get('oracle')} nop={r.get('nop')} {err}")
        print(f"{len(res) - bad}/{len(res)} tasks valid")
        return 1 if bad else 0
    result = runner.run_campaign(
        tasks,
        args.agents.split(",") if args.agents else None,
        tier=args.tier,
        k=args.k,
        upstream=args.upstream,
        budget_usd=args.budget_usd,
        jobs_dir=jobs,
        max_requests_per_trial=args.max_requests_per_trial,
    )
    out = _save(result, args.out)
    print()
    print(report.render_tasks(result))
    print(f"\nspent: {result['spent_usd']:.4f} USD\nraw result: {out}")
    if args.publish:
        print(report.publish(result))
    return 0


def cmd_publish(args: argparse.Namespace) -> int:
    for f in args.results:
        print(report.publish(json.loads(Path(f).read_text())))
    return 0


def cmd_report(args: argparse.Namespace) -> int:
    print(report.write_readme())
    return 0


def cmd_trajectory(args: argparse.Namespace) -> int:
    from flores_bench import trajectory

    errors = trajectory.validate_file(Path(args.file))
    if errors:
        print("\n".join(errors))
        return 1
    events, _ = trajectory.parse_lines(Path(args.file).read_text().splitlines())
    print("valid trajectory/v0")
    print(json.dumps(trajectory.summarize(events), indent=2))
    return 0


def build_parser() -> argparse.ArgumentParser:
    p = argparse.ArgumentParser(prog="flores-bench", description="flores benchmark suite")
    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("doctor", help="show installed agents and tools").set_defaults(fn=cmd_doctor)
    sub.add_parser("build", help="build Go helper binaries").set_defaults(fn=cmd_build)
    pn = sub.add_parser("pinned", help="print pinned npm specs of competitor agents")
    pn.add_argument("--agent", help="print only this agent's pinned version")
    pn.set_defaults(fn=cmd_pinned)

    perf = sub.add_parser("perf", help="layer A performance suites")
    perf.add_argument("suite", choices=["startup", "hello"])
    perf.add_argument("--agents", help="comma-separated subset (default: all in agents.toml)")
    perf.add_argument("--runs", type=int, default=None, help="runs per round")
    perf.add_argument("--warmup", type=int, default=None)
    perf.add_argument("--rounds", type=int, default=3, help="shuffled rounds (default 3)")
    perf.add_argument("--seed", type=int, default=0)
    perf.add_argument("--quick", action="store_true", help="smoke run: few runs, 1 round")
    perf.add_argument("--out", type=Path)
    perf.add_argument("--publish", action="store_true", help="also write into published/")
    perf.add_argument("--enforce", action="store_true", help="exit 1 when an enforced budget fails")
    perf.set_defaults(fn=cmd_perf)

    hb = sub.add_parser("harness", help="layer B: harness behaviour scenarios")
    hb.add_argument("--agents", help="comma-separated subset (default: all in agents.toml)")
    hb.add_argument("--scenarios", help="comma-separated subset of scenario names")
    hb.add_argument("--runs", type=int, default=1, help="runs per scenario; all must pass")
    hb.add_argument("--out", type=Path)
    hb.add_argument("--publish", action="store_true")
    hb.add_argument("--enforce", action="store_true", help="exit 1 if a gated agent regresses (budgets.toml)")
    hb.set_defaults(fn=cmd_harness)

    sc = sub.add_parser("scenarios", help="list layer B scenarios")
    sc.add_argument("--write-docs", action="store_true", help="regenerate docs/harness-scenarios.md")
    sc.set_defaults(fn=cmd_scenarios)

    tk = sub.add_parser("tasks", help="layer C: flores-micro tasks via Harbor")
    tk_sub = tk.add_subparsers(dest="action", required=True)
    for name in ("list", "validate", "run"):
        a = tk_sub.add_parser(name)
        a.add_argument("--tasks", help="comma-separated subset of task names")
        a.set_defaults(fn=cmd_tasks)
        if name == "list":
            a.add_argument("--write-docs", action="store_true", help="regenerate docs/flores-micro.md")
        if name == "validate":
            a.add_argument("--concurrent", type=int, default=4)
        if name == "run":
            a.add_argument("--agents", help="comma-separated subset: claude-code,codex,pi,fx")
            a.add_argument("--tier", default="cheap", help="model tier from suites/tasks/campaign.toml")
            a.add_argument("-k", type=int, default=3, help="attempts per task (pass^k)")
            a.add_argument(
                "--upstream",
                choices=["mock", "gateway", "direct"],
                default="mock",
                help="mock: free dry run; gateway: Cloudflare AI Gateway; direct: provider APIs",
            )
            a.add_argument("--budget-usd", type=float, default=0.0, help="hard spend cap (required unless mock)")
            a.add_argument("--max-requests-per-trial", type=int, default=200)
            a.add_argument("--out", type=Path)
            a.add_argument("--publish", action="store_true")

    cb = sub.add_parser("check-budgets", help="check result files against budgets.toml")
    cb.add_argument("results", nargs="+")
    cb.add_argument("--force", action="store_true", help="enforce even off the reference platform")
    cb.set_defaults(fn=cmd_check_budgets)

    pub = sub.add_parser("publish", help="publish result files into published/")
    pub.add_argument("results", nargs="+")
    pub.set_defaults(fn=cmd_publish)

    sub.add_parser("report", help="regenerate published/README.md").set_defaults(fn=cmd_report)

    tr = sub.add_parser("trajectory", help="trajectory/v0 tools")
    tr_sub = tr.add_subparsers(dest="action", required=True)
    tv = tr_sub.add_parser("validate")
    tv.add_argument("file")
    tv.set_defaults(fn=cmd_trajectory)
    return p


DEFAULTS = {"startup": (30, 5), "hello": (5, 1)}


def main(argv: list[str] | None = None) -> int:
    args = build_parser().parse_args(argv)
    if args.cmd == "perf":
        runs, warmup = DEFAULTS[args.suite]
        args.runs = args.runs if args.runs is not None else runs
        args.warmup = args.warmup if args.warmup is not None else warmup
    return args.fn(args)


if __name__ == "__main__":
    sys.exit(main())
