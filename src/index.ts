import { serve } from "bun";
import index from "./index.html";

const FRESHRSS_URL = process.env.FRESHRSS_URL;
const FRESHRSS_USERNAME = process.env.FRESHRSS_USERNAME;
const FRESHRSS_API_PASSWORD = process.env.FRESHRSS_PASSWORD;

export type FeedItem = {
	id: string;
	crawlTimeMsec: string;
	timestampUsec: string;
	published: number;
	title: string;
	canonical: Array<{
		href: string;
	}>;
	alternate: Array<{
		href: string;
	}>;
	categories: string[];
	origin: {
		streamId: string;
		htmlUrl: string;
		title: string;
	};
	summary: {
		content: string;
	};
	author: string;
};

type FreshRSSResponse = {
	id: string;
	updated: number;
	items: FeedItem[];
	continuation: string;
};

const server = serve({
	routes: {
		// Home page - create snippet form
		"/": index,
		// Fetch a snippet
		"/api/list": {
			async GET() {
				try {
					const authResponse = await fetch(
						`${FRESHRSS_URL}/api/greader.php/accounts/ClientLogin?Email=${FRESHRSS_USERNAME}&Passwd=${FRESHRSS_API_PASSWORD}`,
					);
					const authText = await authResponse.text();

					const match = authText.match(/Auth=(.+)/);

					if (!match || !match[1]) {
						return Response.json(
							{ error: "Authentication failed" },
							{ status: 401 },
						);
					}

					const authToken = match[1].trim();

					const freshResponse = await fetch(
						`${FRESHRSS_URL}/api/greader.php/reader/api/0/stream/contents/reading-list?n=100&r=d`,
						{
							headers: {
								Authorization: `GoogleLogin auth=${authToken}`,
							},
						},
					);

					const freshData: FreshRSSResponse = await freshResponse.json();

					// Map over the items and extract only what you need
					const filteredItems =
						freshData.items?.map((item) => ({
							id: item.id,
							title: item.title,
							published: item.published,
							author: item.author,
							link: item.canonical?.[0]?.href,
							origin: item.origin,
						})) || [];

					return Response.json({
						items: filteredItems,
						total: freshData.items?.length || 0,
						updated: freshData.updated,
						continuation: freshData.continuation,
					});
				} catch (error) {
					return Response.json(
						{ error: `Failed to fetch snippet: ${error}` },
						{ status: 500 },
					);
				}
			},
		},
	},

	development: process.env.NODE_ENV !== "production" && {
		// Enable browser hot reloading in development
		hmr: true,

		// Echo console logs from the browser to the server
		console: true,
	},
});

console.log(`🚀 Server running at ${server.url}`);
