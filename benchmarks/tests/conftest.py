import pytest

from flores_bench.build import build_tools


@pytest.fixture(scope="session", autouse=True)
def _go_tools():
    """Every test session runs against freshly built Go binaries."""
    build_tools()
