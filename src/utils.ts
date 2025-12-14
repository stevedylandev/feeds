import { parseFeed, parseOpml } from "feedsmith";
import type { Rss, Atom } from "feedsmith/types";

type FeedItem = {
	id: string;
	title: string;
	published: number;
	author: string;
	link: string;
	origin: string;
};

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

export async function parse(urls: string[]): Promise<FeedItem[]> {
	const filteredItems: FeedItem[] = [];

	for (const url of urls) {
		const feedResponse = await fetch(url);
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

		filteredItems.push(...items);
	}

	return filteredItems;
}
