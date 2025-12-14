import { parseFeed, parseOpml } from "feedsmith";
import type { Rss, Atom } from "feedsmith/types";
import type { FeedItem } from "./types";

export function parseOpmlFile(opmlContent: string) {
	const opml = parseOpml(opmlContent);
	const urls: string[] = [];
	if (!opml.body || !opml.body.outlines) {
		return urls;
	}
	for (const item of opml.body.outlines) {
		urls.push(item.xmlUrl as string);
	}
	return urls;
}

// Helper function to fetch a single feed with timeout
async function fetchFeedWithTimeout(
	url: string,
	timeoutMs = 5000,
): Promise<FeedItem[]> {
	const controller = new AbortController();
	const timeoutId = setTimeout(() => controller.abort(), timeoutMs);

	try {
		const feedResponse = await fetch(url, { signal: controller.signal });
		clearTimeout(timeoutId);

		const feedContent = await feedResponse.text();
		const { format, feed } = parseFeed(feedContent);

		let items: FeedItem[] = [];

		if (format === "atom") {
			const atomFeed = feed as Atom.Feed<string>;
			items = (atomFeed.entries || []).map((item) => ({
				id: item.id || "",
				title: item.title || "",
				published: Math.floor(
					new Date(item.published || item.updated || "").getTime() / 1000,
				),
				author: item.authors?.[0]?.name || "",
				link: item.links?.[0]?.href || "",
				origin: atomFeed.title || "",
			}));
		} else if (format === "rss" || format === "rdf") {
			const rssFeed = feed as Rss.Feed<string>;
			items = (rssFeed.items || []).map((item) => ({
				id: item.guid?.value || item.link || "",
				title: item.title || "",
				published: Math.floor(new Date(item.pubDate || "").getTime() / 1000),
				author: item.authors?.[0] || item.dc?.creator || "",
				link: item.link || "",
				origin: rssFeed.title || "",
			}));
		}

		return items;
	} catch (error) {
		clearTimeout(timeoutId);
		console.error(`Failed to fetch feed ${url}:`, error);
		return [];
	}
}

export async function parse(urls: string[]): Promise<FeedItem[]> {
	// Fetch all feeds in parallel using Promise.allSettled
	const results = await Promise.allSettled(
		urls.map((url) => fetchFeedWithTimeout(url)),
	);

	// Combine all successful results
	const filteredItems: FeedItem[] = [];
	for (const result of results) {
		if (result.status === "fulfilled") {
			filteredItems.push(...result.value);
		}
	}

	return filteredItems;
}
