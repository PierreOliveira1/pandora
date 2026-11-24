import { afterEach, describe, expect, it, vi } from 'vitest';
import { page } from 'vitest/browser';
import { mount, unmount } from 'svelte';
import type { ServiceView, ToolInfo } from '../types.ts';
import ToolPanel from './ToolPanel.svelte';

let mounted: ReturnType<typeof mount> | null = null;

afterEach(async () => {
	if (mounted) {
		await unmount(mounted);
		mounted = null;
	}
	vi.unstubAllGlobals();
	document.body.innerHTML = '';
});

const service: ServiceView = {
	name: 'todoist',
	kind: 'mcp-http',
	auth: 'oauth',
	expose: 'compact',
	tools_known: true,
	tool_count: 1,
	accounts: [
		{
			name: 'personal',
			description: 'Personal tasks.',
			default: true,
			readonly: false,
			allow: [],
			deny: []
		}
	]
};

function render(tool: ToolInfo): { onEditRules: ReturnType<typeof vi.fn> } {
	const onEditRules = vi.fn();
	const target = document.createElement('div');
	document.body.append(target);

	mounted = mount(ToolPanel, {
		target,
		props: {
			service,
			tool,
			onclose: () => {},
			onremoved: () => {},
			oneditrules: onEditRules
		}
	});

	return { onEditRules };
}

describe('ToolPanel', () => {
	it('a tool that changes data asks for confirmation before running', async () => {
		render({
			name: 'add-tasks',
			description: 'Add a task.',
			inputSchema: {
				type: 'object',
				properties: {
					content: { type: 'string', description: 'Task content.' }
				},
				required: ['content']
			},
			annotations: { readOnlyHint: false }
		});

		await page.getByRole('button', { name: 'Run (changes data)' }).click();

		await expect
			.element(page.getByText('This tool may change data in todoist/personal. Run it?'))
			.toBeVisible();
	});

	it('a JSON result is shown formatted and can switch to its raw text', async () => {
		const raw = '{"z":2,"a":1}';
		vi.stubGlobal(
			'fetch',
			vi.fn(
				async () =>
					new Response(JSON.stringify({ content: [{ type: 'text', text: raw }], isError: false }), {
						status: 200,
						headers: { 'Content-Type': 'application/json' }
					})
			)
		);

		render({
			name: 'get_result',
			description: 'Get a JSON result.',
			inputSchema: { type: 'object', properties: {} },
			annotations: { readOnlyHint: true }
		});

		await page.getByRole('button', { name: 'Run', exact: true }).click();
		await expect.element(page.getByRole('button', { name: 'Formatted' })).toBeVisible();

		await expect.poll(() => document.querySelector('pre')?.textContent).toContain('\n  "z": 2');

		await page.getByRole('button', { name: 'Raw' }).click();
		await expect.poll(() => document.querySelector('pre')?.textContent).toBe(raw);
	});

	it('a blocked call shows the rule and a way to edit it', async () => {
		const message = 'tool get_ad_accounts is blocked because todoist/personal is read-only';
		vi.stubGlobal(
			'fetch',
			vi.fn(
				async () =>
					new Response(JSON.stringify({ error: message }), {
						status: 403,
						headers: { 'Content-Type': 'application/json' }
					})
			)
		);

		const { onEditRules } = render({
			name: 'get_ad_accounts',
			description: 'List ad accounts.',
			inputSchema: { type: 'object', properties: {} },
			annotations: { readOnlyHint: true }
		});

		await page.getByRole('button', { name: 'Run', exact: true }).click();

		await expect.element(page.getByText(message)).toBeVisible();
		await page.getByRole('button', { name: 'Edit rules' }).click();

		expect(onEditRules).toHaveBeenCalledWith('todoist/personal');
	});
});
