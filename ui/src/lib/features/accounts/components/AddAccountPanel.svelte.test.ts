import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import AddAccountPanel from './AddAccountPanel.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	document.body.innerHTML = '';
});

describe('AddAccountPanel', () => {
	it('the add account form asks for a client id when OAuth with my own app is chosen', async () => {
		const target = document.createElement('div');
		document.body.append(target);

		mounted = mount(AddAccountPanel, {
			target,
			props: { services: [], onclose: () => {} }
		});

		await page.getByRole('button', { name: /Remote MCP/ }).click();
		await page.getByRole('radio', { name: 'OAuth with my own app' }).click();

		await expect.element(page.getByRole('textbox', { name: 'Client ID' })).toBeVisible();
	});
});
