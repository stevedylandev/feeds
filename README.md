# feeds

![cover](https://files.stevedylan.dev/feeds-software-demo.png)

*An introduction to RSS*

Small Go server that merges multiple RSS/Atom feeds into one combined view. Paste feed URLs (or a site URL to auto-discover its feed) and see recent items sorted together.

## Run

```sh
go run .
```

Server starts on `:3000`. Override with env vars:

- `HOST` (default `0.0.0.0`)
- `PORT` (default `3000`)
- `BASE_URL` (default `http://localhost:3000`)

## Build

```sh
go build -o feeds .
./feeds
```

## Docker

```sh
docker compose up
```

Or build the image directly from the included `Dockerfile`.

## Usage

- Open `http://localhost:3000`, add feed URLs, and view merged items.
- `GET /?urls=<comma-separated-feed-urls>` — renders merged feed items server-side.
- `GET /api/resolve?url=<url>` — resolves a feed or site URL to its feed(s) as JSON.
- `GET /og.png` — dynamically generated Open Graph image.
- `GET /privacy` — privacy page.

## License

[MIT](LICENSE)
