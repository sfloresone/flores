def slugify(text, max_length=50):
    """Turn `text` into a URL slug.

    Rules:
    - lowercase;
    - accented Latin letters become their plain ASCII letter (á -> a, ñ -> n, ç -> c);
    - every run of characters that are not a-z or 0-9 becomes a single "-";
    - no leading or trailing "-";
    - if the slug is longer than `max_length`, cut it at the last "-" that keeps
      it within `max_length` (never cut a word in half). If a single word is longer
      than `max_length`, cut that word at `max_length`.
    """
    raise NotImplementedError
