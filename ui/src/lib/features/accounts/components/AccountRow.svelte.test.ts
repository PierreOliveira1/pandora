import { afterEach, describe, expect, it } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import type { AccountView, StatusInfo } from '../types.ts';
import AccountRow from './AccountRow.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	document.body.innerHTML = '';
});

function renderRow(status: StatusInfo): void {
	const account: AccountView = {
		name: 'work',
		description: 'Shared projects with the team.',
		default: false,
		readonly: false,
		allow: [],
		deny: []
	};

	const target = document.createElement('div');
	document.body.append(target);

	mounted = mount(AccountRow, {
		target,
		props: {
			serviceName: 'todoist',
			account,
			status,
			statusLoading: false,
			auth: 'oauth',
			loggingIn: false,
			onLogin: () => {},
			onRename: () => {},
			onRemove: () => {},
			onSetDefault: () => {},
			onLogout: () => {},
			onRules: () => {}
		}
	});
}

describe('AccountRow', () => {
	it('an account that needs login offers a log in button', async () => {
		renderRow({
			ref: 'todoist/work',
			ok: false,
			needs_login: true,
			tools: 0,
			latency_ms: 118
		});

		await expect.element(page.getByRole('button', { name: 'Log in' })).toBeVisible();
	});
});
