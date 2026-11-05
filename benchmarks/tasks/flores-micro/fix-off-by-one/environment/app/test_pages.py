import unittest

from pages import paginate


class TestPaginate(unittest.TestCase):
    def test_exact(self):
        self.assertEqual(paginate([1, 2, 3, 4], 2), [[1, 2], [3, 4]])

    def test_remainder(self):
        self.assertEqual(paginate([1, 2, 3, 4, 5], 2), [[1, 2], [3, 4], [5]])

    def test_single(self):
        self.assertEqual(paginate([7], 3), [[7]])


if __name__ == "__main__":
    unittest.main()
