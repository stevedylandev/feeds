import Parser from "rss-parser";

export async function parse(urls: string[]) {
	const parser = new Parser();
	const filteredItems = [];
	for (const url of urls) {
		const feed = await parser.parseURL(url);
		const items =
			feed.items?.map((item) => ({
				id: item.id,
				title: item.title,
				published: Math.floor(
					new Date(item.pubDate as string).getTime() / 1000,
				),
				author: item.author,
				link: item.link,
				origin: item.origin,
			})) || [];
		filteredItems.push(...items);
	}
	return filteredItems;
}
