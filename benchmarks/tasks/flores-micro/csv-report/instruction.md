The CSV files in /app/data contain orders. Write /app/report.json with this shape:

    {"total": <sum of amount>, "by_region": {"<region>": <sum of amount>, ...}}

Only count rows whose status is `paid`. Round every number to 2 decimals.
