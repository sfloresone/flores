import sys
want = ["app.py:3", "app.py:8", "db/models.py:2", "web/views.js:2"]
try:
    got = [l.strip() for l in open("/app/todos.txt") if l.strip()]
except FileNotFoundError:
    sys.exit(1)
print(got)
sys.exit(0 if got == want else 1)
