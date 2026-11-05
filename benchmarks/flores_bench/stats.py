"""Small, dependency-free statistics helpers.

Benchmarks report robust statistics (median and p95) instead of only the mean,
because process timings are right-skewed: a single slow run caused by the OS
should not move the headline number. The 95% confidence interval of the median
comes from a seeded bootstrap so reports are reproducible.
"""

from __future__ import annotations

import math
import random
from collections.abc import Sequence


def percentile(values: Sequence[float], p: float) -> float:
    """Linear-interpolated percentile, p in [0, 100]."""
    if not values:
        raise ValueError("percentile of empty sample")
    xs = sorted(values)
    if len(xs) == 1:
        return float(xs[0])
    k = (len(xs) - 1) * p / 100
    lo, hi = math.floor(k), math.ceil(k)
    return float(xs[lo] + (xs[hi] - xs[lo]) * (k - lo))


def bootstrap_ci(
    values: Sequence[float], stat=lambda xs: percentile(xs, 50), n: int = 2000, seed: int = 0
) -> tuple[float, float]:
    """95% percentile-bootstrap confidence interval for `stat`."""
    if len(values) < 2:
        v = float(values[0]) if values else math.nan
        return (v, v)
    rng = random.Random(seed)
    k = len(values)
    estimates = [stat([values[rng.randrange(k)] for _ in range(k)]) for _ in range(n)]
    return (percentile(estimates, 2.5), percentile(estimates, 97.5))


def summarize(values: Sequence[float]) -> dict[str, float | int | list[float]]:
    """Summary block used in every result file."""
    if not values:
        return {"n": 0}
    n = len(values)
    mean = sum(values) / n
    var = sum((v - mean) ** 2 for v in values) / (n - 1) if n > 1 else 0.0
    lo, hi = bootstrap_ci(values)
    return {
        "n": n,
        "mean": mean,
        "stddev": math.sqrt(var),
        "min": float(min(values)),
        "p50": percentile(values, 50),
        "p95": percentile(values, 95),
        "max": float(max(values)),
        "ci95_p50": [lo, hi],
    }
