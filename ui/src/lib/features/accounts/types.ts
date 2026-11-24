import type { ServiceKind } from '#lib/shared/api/types.ts';

export type {
	AccountPolicy,
	AccountView,
	AddAccountInput,
	AddAccountResponse,
	AuthMode,
	LoginResult,
	Overview,
	ServiceKind,
	ServiceView,
	StatusInfo,
	WebhookInfo
} from '#lib/shared/api/types.ts';

export const serviceKindLabels: Record<ServiceKind, string> = {
	'mcp-http': 'Remote MCP',
	'mcp-stdio': 'Local MCP',
	openapi: 'OpenAPI',
	command: 'Commands',
	http: 'HTTP API'
};

export function serviceKindLabel(kind: ServiceKind): string {
	return serviceKindLabels[kind];
}
