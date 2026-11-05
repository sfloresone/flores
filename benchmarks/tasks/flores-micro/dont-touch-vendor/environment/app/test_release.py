import unittest

from release import next_release


class T(unittest.TestCase):
    def test_patch(self):
        self.assertEqual(next_release("1.2.3"), "1.2.4")

    def test_feature(self):
        self.assertEqual(next_release("1.2.3", feature=True), "1.3.0")

    def test_breaking(self):
        self.assertEqual(next_release("1.2.3", breaking=True), "2.0.0")
