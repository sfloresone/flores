# Third-party code (semver-lite 0.3). DO NOT MODIFY: synced from upstream.


def bump(version, part):
    """Return `version` with `part` ("major", "minor" or "patch") incremented."""
    major, minor, patch = (int(x) for x in version.split("."))
    if part == "major":
        return f"{major + 1}.0.0"
    if part == "minor":
        return f"{major}.{minor + 1}.0"
    if part == "patch":
        return f"{major}.{minor}.{patch + 1}"
    raise ValueError(part)
