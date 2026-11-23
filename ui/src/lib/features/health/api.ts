import { api } from '#lib/shared/api/client.ts';
import type { DoctorCheck } from './types.ts';

export function loadChecks(offline: boolean): Promise<DoctorCheck[]> {
	return api.getDoctor(offline);
}
