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

export type FreshRSSResponse = {
	id: string;
	updated: number;
	items: FeedItem[];
	continuation: string;
};

export type Feed = {
	id: string;
	title: string;
	url: string;
	htmlUrl?: string;
	iconUrl?: string;
};

export type SubscriptionList = {
	subscriptions?: Feed[];
};
