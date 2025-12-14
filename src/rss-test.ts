import { parse } from "./utils";

const feeds = await parse([
	"https://bearblog.dev/discover/feed/",
	"https://bearblog.stevedylan.dev/feed",
]);

console.log(feeds);
