import re
import unicodedata


def slugify(text, max_length=50):
    """See the task description."""
    ascii_text = unicodedata.normalize("NFKD", text).encode("ascii", "ignore").decode()
    slug = re.sub(r"[^a-z0-9]+", "-", ascii_text.lower()).strip("-")
    if len(slug) <= max_length:
        return slug
    cut = slug[: max_length + 1]
    if "-" in cut:
        return cut[: cut.rindex("-")].strip("-")
    return slug[:max_length]
