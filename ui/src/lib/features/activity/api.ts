import { api } from '#lib/shared/api/client.ts';
import type { EventView, LogEntry, Overview, WebhookInfo } from './types.ts';

export function loadOverview(): Promise<Overview> {
	return api.getOverview();
}

export function loadLog(options: { ref?: string; errors?: boolean }): Promise<LogEntry[]> {
	return api.getLog({ n: 200, ...options });
}

export function loadEvents(
	options: { service?: string; account?: string; topic?: string; limit?: number } = {}
): Promise<EventView[]> {
	return api.getEvents(options);
}

export function loadWebhooks(): Promise<WebhookInfo[]> {
	return api.getWebhooks();
}
