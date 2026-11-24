import { api } from '#lib/shared/api/client.ts';
import type { AutostartInfo, DoctorCheck } from './types.ts';

export function loadChecks(offline: boolean): Promise<DoctorCheck[]> {
	return api.getDoctor(offline);
}

export function loadAutostart(): Promise<AutostartInfo> {
	return api.getAutostart();
}

export function saveAutostart(enabled: boolean): Promise<AutostartInfo> {
	return api.setAutostart(enabled);
}
