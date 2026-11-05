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
