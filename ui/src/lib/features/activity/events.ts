import type { JsonValue } from '#lib/shared/api/types.ts';
import type { EventView } from './types.ts';

export function payloadPreview(payload: JsonValue, limit = 140): string {
	const text = typeof payload === 'string' ? payload : JSON.stringify(payload);
	const compact = (text ?? '').replace(/\s+/g, ' ').trim();
	if (compact.length <= limit) return compact;
	return `${compact.slice(0, Math.max(0, limit - 1)).trimEnd()}…`;
}

export function eventTopics(events: EventView[]): string[] {
	const topics = new Set<string>();
	for (const event of events) {
		if (event.topic) topics.add(event.topic);
	}
	return [...topics].sort((left, right) => left.localeCompare(right));
}

export function eventQuery(
	account: string,
	topic: string
): { service?: string; account?: string; topic?: string } {
	const query: { service?: string; account?: string; topic?: string } = {};
	if (account) {
		const [service, name] = account.split('/');
		query.service = service;
		if (name) query.account = name;
	}
	if (topic) query.topic = topic;
	return query;
}
