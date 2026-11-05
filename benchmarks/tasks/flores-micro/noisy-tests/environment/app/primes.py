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
