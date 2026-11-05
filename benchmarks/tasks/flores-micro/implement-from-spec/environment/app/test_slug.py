import unittest

from slug import slugify


class T(unittest.TestCase):
    def test_examples(self):
        self.assertEqual(slugify("Hello, World!"), "hello-world")
        self.assertEqual(slugify("  Árbol de Navidad  "), "arbol-de-navidad")
