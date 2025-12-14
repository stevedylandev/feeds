# Feeds

![cover](https://feeds.software/assets/og.png)

Minimal RSS Feeds

## About

Feeds is a minimal RSS reader that mimics the original experience of RSS. It's just a list of posts. No categories, no marking a post read or unread, and there is no in-app reading. With this approach you have to read the post on the authors personal website and experience it in it's original context. While this may not work well if you have loads of news feeds, I personally love it for [my approach to blogs](https://blogfeeds.net).

This app is also MIT open sourced and designed to be self-hosted; fork the code and change it to your liking!

## Usage

I've built some core logic in `src/utils.ts` where the `parse()` function can take an array of RSS feeds and render them as a list of posts that the `index.html` can render. Thanks to this flexibility there are several built in ways to source RSS feeds. 

### URL Query Param

Once you have the app running you can add the following to the URL to source an RSS feed:

```
?url=https://bearblog.dev/discover/feed/
```

You can also add multiple URLs by using commas to separate them:

```
?urls=https://bearblog.dev/discover/feed/,https://bearblog.stevedylan.dev/feed/
```

> [!TIP]
> You can comment out this functionality inside `index.html` if you want to limit usage to just your selected feeds, or even hardcode an array of feeds.

### OPML File

If you save a `feeds.opml` file in the root of the project the app will automatically source it and fetch the posts for the feeds inside. 

### FreshRSS API

If neither of the above are provided the app will default to using a FreshRSS API instance. Simply run the following command:

```bash
cp .env.sample .env
```

Then fill in the environment variables:

```
FRESHRSS_URL=
FRESHRSS_USERNAME=
FRESHRSS_PASSWORD=
```

## Quickstart

1. Make sure [Bun](https://bun.com) is installed

```bash
bun --version
```

2. Clone and install types

```bash
git clone https://github.com/stevedylandev/feeds
cd feeds
bun install
```

3. Run the dev server

```bash
bun dev
# 🚀 Server running at http://localhost:3000/
```

## Project Structure

```
TBD
```

The architecture is intentionally simple:
- **`index.ts`** - Bun server with file-based routing and JSON API
- **`db.ts`** - Direct SQLite operations with no ORM dependencies
- **HTML files** - Plain HTML with inline JavaScript, no build step required
- **`styles.css`** - Single CSS file with custom font loading
- **SQLite database** - File-based storage for maximum portability

## Deployment

Since Feeds is a basic Bun app, all you need is a server enviornment that can install and run the `start` script.

### Self Hosting

If you are running a VPS or your own hardware like a Raspberry Pi, you can use a basic `systemd` service to manage the instance.

1. Clone the repo and install

```bash
git clone https://github.com/stevedylandev/feeds
cd feeds
bun install
```

2. Create a systemd service

The location of where these files are located might depend on your linux distribution, but most commonly they can be found at `/etc/systemd/system`. Create a new file called `feeds.service` and edit it with `nano` or `vim`.

```bash
cd /etc/systemd/service
touch feeds.service
sudo nano feeds.service
```

Paste in the following code:

```bash
[Unit]
# describe the app
Description=Feeds
# start the app after the network is available
After=network.target

[Service]
# usually you'll use 'simple'
# one of https://www.freedesktop.org/software/systemd/man/systemd.service.html#Type=
Type=simple
# which user to use when starting the app
User=YOURUSER
# path to your application's root directory
WorkingDirectory=/home/YOUR_USER/feeds
# the command to start the app
# requires absolute paths
ExecStart=/home/YOUR_USER/.bun/bin/bun start
# restart policy
# one of {no|on-success|on-failure|on-abnormal|on-watchdog|on-abort|always}
Restart=always

[Install]
# start the app automatically
WantedBy=multi-user.target
```

> [!NOTE]
> Make sure you update the `YOUR_USER` with your own user info, and make sure the paths to `bun` and the `feeds` directory are correct!

3. Start up the service

Run the following commands to enable and start the service

```bash
sudo systemctl enable feeds.service
sudo systemctl start feeds
```

Check and make sure it's working

```bash
sudo systemctl status feeds
```

4. Setup a Tunnel (optional)

From here you have a lot of options of how you may want to access the sipp instance. One easy way to start is to use a Cloudflare tunnel and point it to `http://localhost:3000`.


### Docker

1. Clone the repo

```bash
git clone https://github.com/stevedylandev/feeds
cd feeds
```

2. Build and run the Docker image

```bash
docker build -t feeds .
docker run -p 3000:3000 -v $(pwd)/data:/usr/src/app/data feeds
```

Or use `docker-compose`

```bash
docker-compose up -d
```

### Railway

1. Fork the repo from GitHub to your own account

2. Login to [Railway](https://railway.com) and create a new project

3. Select Feeds from your repos

4. Make sure the start command is `bun run start`
