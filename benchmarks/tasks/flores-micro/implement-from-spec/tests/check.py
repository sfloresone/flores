import sys
sys.path.insert(0, "/app")
from slug import slugify
cases = [
    (("Hello, World!",), "hello-world"),
    (("  Árbol de Navidad  ",), "arbol-de-navidad"),
    (("Ça va? Ñandú!!",), "ca-va-nandu"),
    (("---a---b---",), "a-b"),
    (("C++ & Go: 2026 edition",), "c-go-2026-edition"),
    (("one two three four", 9), "one-two"),
    (("one two three four", 13), "one-two-three"),
    (("supercalifragilistic", 5), "super"),
    (("", ), ""),
    (("Zoë's café", 50), "zoe-s-cafe"),
]
bad = []
for args, want in cases:
    try:
        got = slugify(*args)
    except Exception as e:
        got = repr(e)
    if got != want:
        bad.append((args, want, got))
print(bad)
sys.exit(0 if not bad else 1)
