import { serve } from "bun";
import index from "./index.html";
import about from "./about.html";
import type { SubscriptionList, FreshRSSResponse, Feed } from "./types";
import { parse, parseOpmlFile } from "./utils";

const FRESHRSS_URL = process.env.FRESHRSS_URL;
const FRESHRSS_USERNAME = process.env.FRESHRSS_USERNAME;
const FRESHRSS_API_PASSWORD = process.env.FRESHRSS_PASSWORD;

const server = serve({
	routes: {
		// Home page - create snippet form
		"/": index,
		"/about": about,
		// Serve static assets
		"/assets/*": {
			async GET(req) {
				const url = new URL(req.url);
				const filePath = `src${url.pathname}`;
				try {
					return new Response(Bun.file(filePath));
				} catch (error) {
					console.log(error);
					return new Response("Not Found", { status: 404 });
				}
			},
		},
		// Get subscription feeds
		"/feeds": {
			async GET(request: Request) {
				try {
					const url = new URL(request.url);
					const format = url.searchParams.get("format") || "json";

					// Authenticate
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

					// Get subscription list
					const response = await fetch(
						`${FRESHRSS_URL}/api/greader.php/reader/api/0/subscription/list?output=json`,
						{
							headers: {
								Authorization: `GoogleLogin auth=${authToken}`,
							},
						},
					);

					if (!response.ok) {
						return Response.json(
							{ error: `FreshRSS API error: ${response.statusText}` },
							{ status: 500 },
						);
					}

					const data = (await response.json()) as SubscriptionList;

					if (data.subscriptions) {
						data.subscriptions = data.subscriptions.map(
							({ iconUrl, ...feed }) => feed,
						);
					}

					// Return JSON format
					if (format === "json") {
						return Response.json(data);
					}

					// Return OPML format
					if (format === "opml") {
						const now = new Date().toUTCString();
						const subscriptions = data.subscriptions || [];

						// Helper to escape XML
						const escapeXml = (str: string): string => {
							if (!str) return "";
							return str
								.replace(/&/g, "&amp;")
								.replace(/</g, "&lt;")
								.replace(/>/g, "&gt;")
								.replace(/"/g, "&quot;")
								.replace(/'/g, "&apos;");
						};

						let opml = `<?xml version="1.0" encoding="UTF-8"?>
<opml version="2.0">
  <head>
    <title>Steve's Feeds</title>
    <dateCreated>${now}</dateCreated>
  </head>
  <body>
`;

						// Add all feeds
						subscriptions.forEach((feed: Feed) => {
							opml += `    <outline type="rss" text="${escapeXml(feed.title)}" title="${escapeXml(feed.title)}" xmlUrl="${escapeXml(feed.url)}" htmlUrl="${escapeXml(feed.htmlUrl || "")}" />
`;
						});

						opml += `  </body>
</opml>`;

						return new Response(opml, {
							status: 200,
							headers: {
								"Content-Type": "application/xml",
								"Content-Disposition": 'attachment; filename="feeds.opml"',
							},
						});
					}

					return Response.json(
						{ error: "Invalid format. Use ?format=json or ?format=opml" },
						{ status: 400 },
					);
				} catch (error) {
					return Response.json(
						{ error: `Failed to fetch feeds: ${error}` },
						{ status: 500 },
					);
				}
			},
		},
		// Fetch a snippet
		"/api/list": {
			async GET(request: Request) {
				try {
					// Check for url queries
					// e.g. ?url=https://bearblog.dev/discover/feed/,https://bearblog.stevedylan.dev
					const url = new URL(request.url);
					const urlQuery = url.searchParams.get("url");

					if (urlQuery) {
						// Split by comma and clean up the URLs
						const urls = urlQuery
							.split(",")
							.map((u) => u.trim())
							.filter(Boolean); // Remove empty strings

						const filteredItems = await parse(urls);
						return Response.json({
							items: filteredItems,
						});
					}

					// Check for local OPML File
					// Will be slow if there is a lot of feeds
					const file = Bun.file("feeds.opml");
					const fileExists = await file.exists();
					if (fileExists) {
						const content = await file.text();
						const urls = parseOpmlFile(content);
						if (urls.length === 0) {
							return Response.json(
								{
									error: "No Feeds found in opml file",
								},
								{ status: 400 },
							);
						}
						const filteredItems = await parse(urls);
						return Response.json({
							items: filteredItems,
						});
					}

					// Fallback to FreshRSS Instance
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
							author: item.origin.title,
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

	port: 4555,
	development: process.env.NODE_ENV !== "production" && {
		// Enable browser hot reloading in development
		hmr: true,

		// Echo console logs from the browser to the server
		console: true,
	},
});

console.log(`🚀 Server running at ${server.url}`);
