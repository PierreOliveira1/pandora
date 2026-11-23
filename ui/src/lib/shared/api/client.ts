import { markSessionExpired } from './session.svelte.ts';
import type {
	AccountPolicy,
	AddAccountInput,
	AddAccountResponse,
	CallToolInput,
	CallToolResult,
	CommandToolInput,
	DoctorCheck,
	InstallReport,
	IntegrationView,
	LoginResult,
	LogEntry,
	Overview,
	StatusInfo,
	ToolCatalog
} from './types.ts';

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'DELETE';

export class ApiError extends Error {
	readonly status: number;

	constructor(message: string, status: number) {
		super(message);
		this.name = 'ApiError';
		this.status = status;
	}
}

export function errorMessage(error: unknown): string {
	if (error instanceof Error && error.message.trim()) return error.message;
	return 'Something went wrong.';
}

async function request<T>(method: HttpMethod, path: string, body?: unknown): Promise<T> {
	if (import.meta.env.VITE_MOCK === '1') {
		const { mockRequest } = await import('./mock.ts');
		return (await mockRequest(method, path, body)) as T;
	}

	const headers: Record<string, string> = { Accept: 'application/json' };
	if (method !== 'GET') {
		headers['X-Pandora'] = '1';
		headers['Content-Type'] = 'application/json';
	}

	const response = await fetch(path, {
		method,
		credentials: 'same-origin',
		headers,
		body: body === undefined ? undefined : JSON.stringify(body)
	});

	if (!response.ok) {
		if (response.status === 401) markSessionExpired();

		let message = `Request failed with status ${response.status}.`;
		try {
			const payload = (await response.json()) as { error?: unknown };
			if (typeof payload.error === 'string' && payload.error.trim()) message = payload.error;
		} catch {
			message = `Request failed with status ${response.status}.`;
		}
		throw new ApiError(message, response.status);
	}

	if (response.status === 204) return undefined as T;

	const payload = (await response.json()) as T;
	return payload;
}

function accountPath(service: string, account: string): string {
	return `/api/accounts/${encodeURIComponent(service)}/${encodeURIComponent(account)}`;
}

export const api = {
	getOverview(): Promise<Overview> {
		return request<Overview>('GET', '/api/overview');
	},

	getStatus(ref?: string): Promise<StatusInfo[]> {
		const query = ref ? `?ref=${encodeURIComponent(ref)}` : '';
		return request<StatusInfo[]>('GET', `/api/status${query}`);
	},

	addAccount(input: AddAccountInput): Promise<AddAccountResponse> {
		return request<AddAccountResponse>('POST', '/api/accounts', input);
	},

	deleteAccount(service: string, account: string): Promise<void> {
		return request<void>('DELETE', accountPath(service, account));
	},

	deleteService(service: string): Promise<void> {
		return request<void>('DELETE', `/api/services/${encodeURIComponent(service)}`);
	},

	renameAccount(service: string, account: string, name: string): Promise<void> {
		return request<void>('POST', `${accountPath(service, account)}/rename`, { name });
	},

	renameService(service: string, name: string): Promise<void> {
		return request<void>('POST', `/api/services/${encodeURIComponent(service)}/rename`, { name });
	},

	setDefaultAccount(service: string, account: string): Promise<void> {
		return request<void>('POST', `${accountPath(service, account)}/default`);
	},

	updateDescription(ref: string, text: string): Promise<void> {
		return request<void>('PUT', '/api/descriptions', { ref, text });
	},

	startLogin(service: string, account: string): Promise<{ login_id: string }> {
		return request<{ login_id: string }>('POST', `${accountPath(service, account)}/login`);
	},

	getLogin(loginId: string): Promise<LoginResult> {
		return request<LoginResult>('GET', `/api/logins/${encodeURIComponent(loginId)}`);
	},

	logout(service: string, account: string): Promise<void> {
		return request<void>('POST', `${accountPath(service, account)}/logout`);
	},

	getTools(service: string, refresh = false): Promise<ToolCatalog> {
		const query = refresh ? '?refresh=1' : '';
		return request<ToolCatalog>(
			'GET',
			`/api/services/${encodeURIComponent(service)}/tools${query}`
		);
	},

	callTool(input: CallToolInput): Promise<CallToolResult> {
		return request<CallToolResult>('POST', '/api/call', input);
	},

	getPolicy(service: string, account: string): Promise<AccountPolicy> {
		return request<AccountPolicy>('GET', `${accountPath(service, account)}/policy`);
	},

	setPolicy(service: string, account: string, policy: AccountPolicy): Promise<AccountPolicy> {
		return request<AccountPolicy>('PUT', `${accountPath(service, account)}/policy`, policy);
	},

	getLog(options: { n?: number; ref?: string; errors?: boolean } = {}): Promise<LogEntry[]> {
		const query = new URLSearchParams({ n: String(options.n ?? 200) });
		if (options.ref) query.set('ref', options.ref);
		if (options.errors) query.set('errors', '1');
		return request<LogEntry[]>('GET', `/api/log?${query.toString()}`);
	},

	getIntegrations(): Promise<IntegrationView[]> {
		return request<IntegrationView[]>('GET', '/api/integrations');
	},

	installIntegration(client: string, mcp = false): Promise<InstallReport[]> {
		return request<InstallReport[]>(
			'POST',
			`/api/integrations/${encodeURIComponent(client)}/install`,
			{
				mcp
			}
		);
	},

	uninstallIntegration(client: string): Promise<InstallReport[]> {
		return request<InstallReport[]>('DELETE', `/api/integrations/${encodeURIComponent(client)}`);
	},

	getDoctor(offline = false): Promise<DoctorCheck[]> {
		const query = offline ? '?offline=1' : '';
		return request<DoctorCheck[]>('GET', `/api/doctor${query}`);
	},

	addCommandTool(service: string, tool: CommandToolInput): Promise<void> {
		return request<void>(
			'POST',
			`/api/services/${encodeURIComponent(service)}/command-tools`,
			tool
		);
	},

	removeCommandTool(service: string, name: string): Promise<void> {
		return request<void>(
			'DELETE',
			`/api/services/${encodeURIComponent(service)}/command-tools/${encodeURIComponent(name)}`
		);
	}
};
