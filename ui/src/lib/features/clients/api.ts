import { api } from '#lib/shared/api/client.ts';
import type { InstallReport, IntegrationView } from './types.ts';

export function loadIntegrations(): Promise<IntegrationView[]> {
	return api.getIntegrations();
}

export function install(client: string, mcp = false): Promise<InstallReport[]> {
	return api.installIntegration(client, mcp);
}

export function uninstall(client: string): Promise<InstallReport[]> {
	return api.uninstallIntegration(client);
}
