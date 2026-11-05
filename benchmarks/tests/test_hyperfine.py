from flores_bench.hyperfine import _join, parse_export

V1 = {
    "results": [
        {
            "command": "fx help",
            "times": [0.001, 0.002],
            "user": 0.0005,
            "system": 0.0002,
            "memory_usage_byte": [1000, 2000],
            "exit_codes": [0, 0],
        }
    ]
}

V2 = {
    "schema_version": "2",
    "results": [
        {
            "command": "/bin/true",
            "name": "baseline",
            "measurements": [
                {
                    "time_wall_clock": {"value": 0.0017, "unit": "second"},
                    "time_user": {"value": 0.0005, "unit": "second"},
                    "time_system": {"value": 0.0004, "unit": "second"},
                    "memory_peak_resident": {"value": 1196032.0, "unit": "byte"},
                    "exit_code": 0,
                }
            ],
        }
    ],
}


def test_parse_hyperfine_1x():
    cols = parse_export(V1)["fx help"]
    assert cols["wall_s"] == [0.001, 0.002]
    assert cols["rss_bytes"] == [1000, 2000]
    assert cols["user_s"] == [0.0005, 0.0005]


def test_parse_hyperfine_2x_uses_command_name():
    out = parse_export(V2)
    assert list(out) == ["baseline"]
    assert out["baseline"]["rss_bytes"] == [1196032.0]
    assert out["baseline"]["exit_codes"] == [0]


def test_join_quotes_only_when_needed():
    assert _join(["/usr/bin/pi", "--version"]) == "/usr/bin/pi --version"
    assert _join(["x", "hello world", "it's"]) == "x 'hello world' 'it'\\''s'"
