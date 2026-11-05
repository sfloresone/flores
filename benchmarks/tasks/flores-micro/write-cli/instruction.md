Create an executable `/app/bin/wcl` (any language available in the container)
that counts lines, words and bytes of the files given as arguments:

- `wcl FILE...` prints one line per file: `<lines> <words> <bytes> <path>`,
  then, if there is more than one file, a final line `<lines> <words> <bytes> total`.
- `wcl --json FILE...` prints a single JSON object mapping each path to
  `{"lines": n, "words": n, "bytes": n}` (no total).
- If a file does not exist, print `wcl: <path>: not found` to stderr, skip it,
  and exit with status 1 after processing the rest (otherwise exit 0).
