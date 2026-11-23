import { api } from '#lib/shared/api/client.ts';
import type { LogEntry, Overview } from './types.ts';

export function loadOverview(): Promise<Overview> {
	return api.getOverview();
}

export function loadLog(options: { ref?: string; errors?: boolean }): Promise<LogEntry[]> {
	return api.getLog({ n: 200, ...options });
}
