import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import type { AccountView } from '../types.ts';
import AccountValuesPanel from './AccountValuesPanel.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	document.body.innerHTML = '';
});

describe('AccountValuesPanel', () => {
	it('never shows the value of a secret', async () => {
		const account: AccountView = {
			name: 'store',
			default: true,
			readonly: false,
			allow: [],
			deny: [],
			env: { STORE_ID: '42', SHOP_TOKEN: 'leaked-value' },
			secret_env: ['SHOP_TOKEN'],
			secret_headers: []
		};
		const target = document.createElement('div');
		document.body.append(target);

		mounted = mount(AccountValuesPanel, {
			target,
			props: {
				serviceName: 'shop-api',
				account,
				onclose: () => {},
				onchanged: async () => {}
			}
		});

		await expect.element(page.getByText('STORE_ID')).toBeVisible();
		await expect.element(page.getByText('42')).toBeVisible();
		await expect.element(page.getByText('SHOP_TOKEN')).toBeVisible();
		await expect.element(page.getByText('leaked-value')).not.toBeInTheDocument();
	});
});
