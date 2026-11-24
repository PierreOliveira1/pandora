import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import type { AddAccountResponse } from '../types.ts';
import AddAccountPanel from './AddAccountPanel.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	vi.unstubAllGlobals();
	document.body.innerHTML = '';
});

function renderPanel(): void {
	const target = document.createElement('div');
	document.body.append(target);

	mounted = mount(AddAccountPanel, {
		target,
		props: { services: [], onclose: () => {} }
	});
}

function addedResponse(): Response {
	const result: AddAccountResponse = {
		ref: 'example/default',
		status: {
			ref: 'example/default',
			ok: true,
			needs_login: false,
			tools: 0,
			latency_ms: 0
		}
	};

	return new Response(JSON.stringify(result), {
		status: 200,
		headers: { 'Content-Type': 'application/json' }
	});
}

describe('AddAccountPanel', () => {
	it('the add account form asks for a client id when OAuth with my own app is chosen', async () => {
		renderPanel();

		await page.getByRole('button', { name: /Remote MCP/ }).click();
		await page.getByRole('radio', { name: 'OAuth with my own app' }).click();

		await expect.element(page.getByRole('textbox', { name: 'Client ID' })).toBeVisible();
	});

	it('creating an HTTP service sends its URLs and account values', async () => {
		const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
			addedResponse()
		);
		vi.stubGlobal('fetch', fetchMock);
		renderPanel();

		await page.getByRole('button', { name: /^HTTP API/ }).click();
		await page.getByRole('textbox', { name: 'Base URL' }).fill('https://api.example.com/v1');

		await page.getByRole('button', { name: 'Add value' }).click();
		await page.getByRole('button', { name: 'Add value' }).click();

		await page.getByRole('textbox', { name: 'Variable name' }).nth(0).fill('STORE_ID');
		await page.getByRole('textbox', { name: 'Variable value' }).nth(0).fill('42');
		await page.getByRole('textbox', { name: 'Variable name' }).nth(1).fill('SHOP_TOKEN');
		await page.getByRole('textbox', { name: 'Variable value' }).nth(1).fill('s3cr3t');
		await page.getByRole('switch', { name: 'Secret' }).nth(1).click();

		await page.getByRole('button', { name: 'Add account', exact: true }).click();

		await vi.waitFor(() => {
			expect(fetchMock).toHaveBeenCalled();
		});

		const call = fetchMock.mock.calls.find(
			([input, init]) => String(input) === '/api/accounts' && init?.method === 'POST'
		);
		expect(call).toBeDefined();

		const body = JSON.parse(String(call?.[1]?.body)) as {
			service: string;
			account: string;
			kind: string;
			urls: Record<string, string>;
			env?: Record<string, string>;
			secret_env?: Record<string, string>;
		};

		expect(body.service).toBe('example');
		expect(body.account).toBe('default');
		expect(body.kind).toBe('http');
		expect(body.urls).toEqual({ api: 'https://api.example.com/v1' });
		expect(body.env).toEqual({ STORE_ID: '42' });
		expect(body.secret_env).toEqual({ SHOP_TOKEN: 's3cr3t' });
	});

	it('an HTTP URL outside localhost is refused before sending', async () => {
		const fetchMock = vi.fn(async (_input: RequestInfo | URL, _init?: RequestInit) =>
			addedResponse()
		);
		vi.stubGlobal('fetch', fetchMock);
		renderPanel();

		await page.getByRole('button', { name: /^HTTP API/ }).click();
		await page.getByRole('textbox', { name: 'Base URL' }).fill('http://example.com/v1');
		await page.getByRole('button', { name: 'Add account', exact: true }).click();

		await expect
			.element(page.getByText('Use https for this URL; http works only for localhost.'))
			.toBeVisible();
		expect(fetchMock).not.toHaveBeenCalled();
	});
});
