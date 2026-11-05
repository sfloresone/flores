# inventory-service

## Configuration

Copy `config/settings.example.toml` to `config/settings.toml` and edit it:

- `environment` must be `"production"` for deployed instances.
- `port` must be between 1024 and 65535.
- `database_url` uses the `sqlite:///` scheme; production uses `sqlite:///data/inventory.db`.

Check the configuration with `python main.py --check`.
