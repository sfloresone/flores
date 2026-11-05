#!/usr/bin/env bash
# Rejects pull requests that would put build artefacts or very large files into
# git history. It checks every blob introduced by any commit in BASE..HEAD, not
# only the final diff: a file added and deleted inside a PR still lands in
# main's history with a merge commit (that is how benchmarks/bin/measure, a
# 3.9 MB local build, got in with 43c4381).
#
# Usage: check-new-blobs.sh BASE HEAD    (MAX_BYTES overrides the 1 MiB limit)
set -euo pipefail

base=${1:?usage: check-new-blobs.sh BASE HEAD}
head=${2:?usage: check-new-blobs.sh BASE HEAD}
max=${MAX_BYTES:-1048576}
bad=0

# Native binaries by magic number; images and other data are allowed below the size limit.
kind_of() {
	local magic
	magic=$(git cat-file blob "$1" | head -c 4 | od -An -tx1 | tr -d ' \n') || true
	case $magic in
	7f454c46) echo "ELF binary" ;;
	feedface | feedfacf | cefaedfe | cffaedfe) echo "Mach-O binary" ;;
	cafebabe | bebafeca) echo "universal Mach-O binary or Java class" ;;
	4d5a*) echo "Windows PE executable" ;;
	213c6172) echo "static library (ar archive)" ;;
	esac
}

while read -r type sha size path; do
	[ "$type" = blob ] || continue
	problem=$(kind_of "$sha")
	if [ -z "$problem" ] && [ "$size" -gt "$max" ]; then
		problem="$((size / 1024)) KiB, over the $((max / 1024)) KiB limit"
	fi
	if [ -n "$problem" ]; then
		echo "::error file=${path}::${path}: ${problem} (blob ${sha:0:10}). Build outputs go to ignored dirs such as benchmarks/.bin/; if it was committed and removed in a later commit, it is still in this PR's history: drop it with an interactive rebase."
		bad=1
	fi
done < <(git rev-list --objects "$base..$head" | git cat-file --batch-check='%(objecttype) %(objectname) %(objectsize) %(rest)')

if [ "$bad" = 0 ]; then
	echo "OK: no native binaries or blobs over $((max / 1024)) KiB in $base..$head"
fi
exit "$bad"
