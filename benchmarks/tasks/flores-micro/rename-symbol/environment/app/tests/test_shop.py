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
