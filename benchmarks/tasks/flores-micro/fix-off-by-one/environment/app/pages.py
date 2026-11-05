def paginate(items, page_size):
    """Split items into pages of at most page_size elements."""
    pages = []
    for start in range(0, len(items) - 1, page_size):
        pages.append(items[start : start + page_size])
    return pages
