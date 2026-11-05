.PHONY: help build test lint doctor bench bench-quick harness publish

help:
	@echo "make test         Go + Python tests and lint"
	@echo "make doctor       installed agents/tools and versions"
	@echo "make bench-quick  smoke run of every perf suite (seconds)"
	@echo "make bench        full perf run (minutes), raw results in results/"
	@echo "make harness      layer B behaviour matrix for every agent (minutes)"
	@echo "make publish      publish the latest raw results into published/"

build:
	uv run flores-bench build

test: lint
	go vet ./... && go test ./...
	uv run pytest -q

lint:
	gofmt -l tools | (! grep .)
	uv run ruff check . && uv run ruff format --check .
	uv run flores-bench scenarios --write-docs >/dev/null && git diff --exit-code -- docs/harness-scenarios.md

doctor:
	uv run flores-bench doctor

bench-quick:
	uv run flores-bench perf startup --quick
	uv run flores-bench perf hello --quick

bench:
	uv run flores-bench perf startup --out results/startup.json
	uv run flores-bench perf hello --out results/hello.json

harness:
	uv run flores-bench harness --out results/harness.json

publish:
	uv run flores-bench publish results/startup.json results/hello.json $(wildcard results/harness.json)
