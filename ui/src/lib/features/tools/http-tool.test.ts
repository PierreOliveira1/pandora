import { describe, expect, it } from 'vitest';
import type { ServiceView } from './types.ts';
import {
	buildHttpToolInput,
	findSecretEnv,
	formatToolText,
	requestPreview,
	type HttpToolForm
} from './http-tool.ts';

function form(overrides: Partial<HttpToolForm> = {}): HttpToolForm {
	return {
		name: 'list-orders',
		description: '',
		url: 'api',
		method: 'POST',
		path: '/orders/{{id}}',
		query: [{ key: 'limit', value: '{{limit}}' }],
		headers: [],
		bodyMode: 'none',
		body: [],
		parameters: [
			{ name: 'id', type: 'string', required: true, description: '' },
			{ name: 'limit', type: 'integer', required: false, description: '' }
		],
		readonly: false,
		timeoutSec: 60,
		...overrides
	};
}

const service: ServiceView = {
	name: 'shop-api',
	kind: 'http',
	auth: 'none',
	expose: 'compact',
	urls: {
		api: 'https://shop.example.com/v1',
		backup: 'https://backup.example.com/v1'
	},
	tools_known: true,
	tool_count: 0,
	accounts: []
};

describe('http tools', () => {
	it('builds an HTTP input with the method, path and query', () => {
		const input = buildHttpToolInput(form());

		expect(input.http?.method).toBe('POST');
		expect(input.http?.path).toBe('/orders/{{id}}');
		expect(input.http?.query).toEqual({ limit: '{{limit}}' });
	});

	it('does not send a body for GET', () => {
		const input = buildHttpToolInput(
			form({
				method: 'GET',
				bodyMode: 'json',
				body: [{ key: 'reason', value: '{{reason}}' }],
				readonly: false
			})
		);

		expect(input.http?.json).toBeUndefined();
		expect(input.http?.form).toBeUndefined();
		expect(input.readonly).toBe(true);
	});

	it('reports a secret environment variable used in the tool', () => {
		expect(findSecretEnv(['/orders/{{env.SHOP_TOKEN}}'], ['SHOP_TOKEN'])).toBe('SHOP_TOKEN');
		expect(findSecretEnv(['/orders/{{env.STORE_ID}}'], ['SHOP_TOKEN'])).toBeNull();
	});

	it('resolves the request preview base by URL name', () => {
		expect(
			requestPreview(service, {
				url: 'api',
				method: 'GET',
				path: '/items/{{id}}',
				query: { limit: '{{limit}}' }
			})
		).toBe('GET https://shop.example.com/v1/items/{{id}}?limit={{limit}}');
	});

	it('formats JSON and leaves plain text as it is', () => {
		expect(formatToolText('{"total":2,"orders":[]}')).toEqual({
			json: true,
			formatted: '{\n  "total": 2,\n  "orders": []\n}'
		});
		expect(formatToolText('2 orders found')).toEqual({
			json: false,
			formatted: '2 orders found'
		});
	});
});
