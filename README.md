# feeds

Small Go server that merges multiple RSS/Atom feeds into one combined view. Paste feed URLs (or a site URL to auto-discover its feed) and see recent items sorted together.

## Run

```sh
go run .
```

Server starts on `:3000` (override with `PORT`/`HOST`/`BASE_URL` env vars).

## Build

```sh
go build -o feeds .
./feeds
```

## Usage

- Open `http://localhost:3000`, add feed URLs, and view merged items.
- `GET /?urls=<comma-separated-feed-urls>` — renders merged feed items server-side.
- `GET /api/resolve?url=<url>` — resolves a feed or site URL to its feed(s) as JSON.

## License

[MIT](LICENSE)
