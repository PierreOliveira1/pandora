import { api, ApiError } from '#lib/shared/api/client.ts';
import type {
	AccountPolicy,
	AddAccountInput,
	AddAccountResponse,
	LoginResult,
	Overview,
	StatusInfo,
	WebhookInfo
} from './types.ts';

export function loadOverview(): Promise<Overview> {
	return api.getOverview();
}

export function loadStatus(ref?: string): Promise<StatusInfo[]> {
	return api.getStatus(ref);
}

export function loadWebhooks(): Promise<WebhookInfo[]> {
	return api.getWebhooks();
}

export function removeWebhook(service: string, account: string): Promise<void> {
	return api.deleteWebhook(service, account);
}

export function addAccount(input: AddAccountInput): Promise<AddAccountResponse> {
	return api.addAccount(input);
}

export function removeAccount(service: string, account: string): Promise<void> {
	return api.deleteAccount(service, account);
}

export function removeService(service: string): Promise<void> {
	return api.deleteService(service);
}

export function renameAccount(service: string, account: string, name: string): Promise<void> {
	return api.renameAccount(service, account, name);
}

export function renameService(service: string, name: string): Promise<void> {
	return api.renameService(service, name);
}

export function setServiceUrl(service: string, name: string, url: string): Promise<void> {
	return api.setServiceUrl(service, name, url);
}

export function removeServiceUrl(service: string, name: string): Promise<void> {
	return api.removeServiceUrl(service, name);
}

export function setServiceHeader(service: string, name: string, template: string): Promise<void> {
	return api.setServiceHeader(service, name, template);
}

export function removeServiceHeader(service: string, name: string): Promise<void> {
	return api.removeServiceHeader(service, name);
}

export function setAccountEnv(
	service: string,
	account: string,
	name: string,
	value: string
): Promise<void> {
	return api.setAccountEnv(service, account, name, value);
}

export function unsetAccountEnv(service: string, account: string, name: string): Promise<void> {
	return api.unsetAccountEnv(service, account, name);
}

export function makeDefaultAccount(service: string, account: string): Promise<void> {
	return api.setDefaultAccount(service, account);
}

export function saveDescription(ref: string, text: string): Promise<void> {
	return api.updateDescription(ref, text);
}

export function logoutAccount(service: string, account: string): Promise<void> {
	return api.logout(service, account);
}

export function loadPolicy(service: string, account: string): Promise<AccountPolicy> {
	return api.getPolicy(service, account);
}

export function savePolicy(
	service: string,
	account: string,
	policy: AccountPolicy
): Promise<AccountPolicy> {
	return api.setPolicy(service, account, policy);
}

export function startLogin(service: string, account: string): Promise<{ login_id: string }> {
	return api.startLogin(service, account);
}

function sleep(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

export async function waitForLogin(loginId: string): Promise<void> {
	const deadline = Date.now() + 6 * 60 * 1000;

	while (Date.now() < deadline) {
		const result: LoginResult = await api.getLogin(loginId);
		if (result.state === 'done') return;
		if (result.state === 'error') {
			throw new ApiError(result.error ?? 'The login did not finish.', 400);
		}
		await sleep(1000);
	}

	throw new ApiError('The login timed out. Try again from the account row.', 408);
}
