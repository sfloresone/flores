import subprocess, sys
def git(*a):
    return subprocess.run(["git", "-C", "/app", *a], capture_output=True, text=True).stdout
msgs = git("log", "--format=%s").splitlines()
tree = git("grep", "-n", "recieve", "HEAD", "--", "src")
changelog_ok = "recieve" in git("show", "HEAD:CHANGELOG.md")
clean = git("status", "--porcelain").strip() == ""
ok = len(msgs) == 2 and msgs[0].startswith("fix: ") and not tree and changelog_ok and clean
print(msgs, repr(tree), changelog_ok, clean)
sys.exit(0 if ok else 1)
