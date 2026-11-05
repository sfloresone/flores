import unittest

from service import Service, wait_until_ready


class T(unittest.TestCase):
    def test_ready(self):
        self.assertEqual(wait_until_ready(Service(3)), 3)

    def test_already_ready(self):
        self.assertEqual(wait_until_ready(Service(0)), 0)
