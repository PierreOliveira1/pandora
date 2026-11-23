import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import ClientsPage from './ClientsPage.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	vi.unstubAllGlobals();
	document.body.innerHTML = '';
});

describe('ClientsPage', () => {
	it('a client that is not installed offers to connect it', async () => {
		vi.stubGlobal(
			'fetch',
			vi.fn(
				async () =>
					new Response(
						JSON.stringify([
							{
								client: 'codex',
								detected: true,
								supports_skill: true,
								mode: ''
							}
						]),
						{
							status: 200,
							headers: { 'Content-Type': 'application/json' }
						}
					)
			)
		);

		const target = document.createElement('div');
		document.body.append(target);

		mounted = mount(ClientsPage, { target });

		await expect.element(page.getByRole('heading', { name: 'Codex' })).toBeVisible();
		await expect.element(page.getByRole('button', { name: 'Connect', exact: true })).toBeVisible();
	});
});
