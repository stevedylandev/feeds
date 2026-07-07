// Builder UI for the root page. All persistent state lives in the URL —
// the pending list here only exists until the user presses Go.

// Example feeds. Favicon is optional; when omitted it falls back to the
// site's /favicon.ico.
const EXAMPLE_FEEDS = [
  {
    title: "NYT Top Stories",
    url: "https://rss.nytimes.com/services/xml/rss/nyt/HomePage.xml",
    favicon: "https://www.nytimes.com/favicon.ico",
  },
  {
    title: "NPR",
    url: "https://feeds.npr.org/1002/rss.xml",
    favicon: "https://media.npr.org/chrome/favicon/favicon-180x180.png"
  },
  {
    title: "Hacker News",
    url: "https://news.ycombinator.com/rss",
    favicon: "https://news.ycombinator.com/y18.svg",
  },
  {
    title: "NASA Images",
    url: "https://www.nasa.gov/feeds/iotd-feed/",
    favicon: "https://www.nasa.gov/wp-content/plugins/nasa-hds-core-setup/assets/favicons/apple-touch-icon-57x57.png"
  },
  {
    title: "Bubbles",
    url: "https://bubbles.town/feed",
    favicon: "https://bubbles.town/static/favicon-32.png",
  },
  {
    title: "Art Calendar",
    url: "https://easel.stevedylan.dev/feed.xml",
    favicon: "https://easel.stevedylan.dev/static/favicon.ico"
  }
];

function fallbackFavicon(feedURL) {
  try {
    return new URL(feedURL).origin + "/favicon.ico";
  } catch {
    return "";
  }
}

function faviconImg(src) {
  const img = document.createElement("img");
  img.className = "favicon";
  img.width = 16;
  img.height = 16;
  img.alt = "";
  img.src = src;
  img.addEventListener("error", () => img.remove());
  return img;
}

const input = document.getElementById("feed-input");
const addButton = document.getElementById("add-button");
const goButton = document.getElementById("go-button");
const status = document.getElementById("status");
const examples = document.getElementById("examples");
const pendingList = document.getElementById("pending");

const pending = [];

function setStatus(text, spinning) {
  status.textContent = text;
  status.classList.toggle("hidden", !text && !spinning);
  status.classList.toggle("spinner", Boolean(spinning));
}

function renderPending() {
  pendingList.innerHTML = "";
  for (const feed of pending) {
    const li = document.createElement("li");
    const favicon = feed.favicon || fallbackFavicon(feed.url);
    if (favicon) li.appendChild(faviconImg(favicon));
    const label = document.createElement("span");
    label.className = "pending-label";
    label.textContent = feed.title || feed.url;
    const url = document.createElement("span");
    url.className = "pending-url";
    url.textContent = feed.url;
    const remove = document.createElement("button");
    remove.className = "link-button danger";
    remove.textContent = "×";
    remove.title = "Remove feed";
    remove.addEventListener("click", () => {
      pending.splice(pending.indexOf(feed), 1);
      renderPending();
    });
    li.append(label, url, remove);
    pendingList.appendChild(li);
  }
  goButton.disabled = pending.length === 0;
}

function addFeed(url, title, favicon) {
  if (pending.some((f) => f.url === url)) {
    setStatus("already added", false);
    return;
  }
  pending.push({ url, title, favicon });
  setStatus("", false);
  renderPending();
}

async function resolve() {
  const value = input.value.trim();
  if (!value) return;
  input.disabled = true;
  addButton.disabled = true;
  setStatus("finding feed", true);
  try {
    const resp = await fetch("/api/resolve?url=" + encodeURIComponent(value));
    const body = await resp.json();
    if (!resp.ok) {
      setStatus(body.error || "could not resolve feed", false);
      return;
    }
    for (const feed of body.feeds) {
      addFeed(feed.url, feed.title, feed.favicon);
    }
    input.value = "";
  } catch {
    setStatus("could not resolve feed", false);
  } finally {
    input.disabled = false;
    addButton.disabled = false;
    input.focus();
  }
}

addButton.addEventListener("click", resolve);
input.addEventListener("keydown", (e) => {
  if (e.key === "Enter") {
    e.preventDefault();
    resolve();
  }
});

goButton.addEventListener("click", () => {
  if (pending.length === 0) return;
  location.href = "/?url=" + pending.map((f) => encodeURIComponent(f.url)).join(",");
});

for (const feed of EXAMPLE_FEEDS) {
  const chip = document.createElement("button");
  chip.className = "chip";
  const favicon = feed.favicon || fallbackFavicon(feed.url);
  if (favicon) chip.appendChild(faviconImg(favicon));
  chip.appendChild(document.createTextNode(feed.title));
  chip.title = feed.url;
  chip.addEventListener("click", () => addFeed(feed.url, feed.title, feed.favicon));
  examples.appendChild(chip);
}
