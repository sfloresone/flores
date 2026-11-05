from vendor.semver_lite import bump


def next_release(current, breaking=False, feature=False):
    if breaking:
        return bump("major", current)
    if feature:
        return bump("minor", current)
    return bump("patch", current)
