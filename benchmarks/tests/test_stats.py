import math

import pytest

from flores_bench.stats import bootstrap_ci, percentile, summarize


def test_percentile_interpolates():
    assert percentile([1, 2, 3, 4], 50) == 2.5
    assert percentile([5], 95) == 5
    assert percentile([1, 2, 3, 4, 5], 0) == 1
    assert percentile([1, 2, 3, 4, 5], 100) == 5
    with pytest.raises(ValueError):
        percentile([], 50)


def test_summarize_shape_and_values():
    s = summarize([10, 11, 12, 13, 100])
    assert s["n"] == 5
    assert s["p50"] == 12
    assert s["min"] == 10 and s["max"] == 100
    lo, hi = s["ci95_p50"]
    assert lo <= s["p50"] <= hi
    assert summarize([]) == {"n": 0}


def test_bootstrap_is_reproducible():
    xs = [1.0, 2.0, 3.0, 4.0, 50.0, 6.0, 7.0]
    assert bootstrap_ci(xs) == bootstrap_ci(xs)
    lo, hi = bootstrap_ci([3.0])
    assert lo == hi == 3.0
    assert all(math.isfinite(v) for v in bootstrap_ci(xs))
