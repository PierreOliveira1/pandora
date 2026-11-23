import { describe, expect, it } from 'vitest';
import { suggestNameFromUrl, suggestServiceName } from './suggest-name.ts';

describe('service name suggestion', () => {
	it('a service name is suggested from the URL or the command', () => {
		expect(suggestServiceName({ kind: 'mcp-http', url: 'https://ai.todoist.net/mcp' })).toBe(
			'todoist'
		);
		expect(suggestServiceName({ kind: 'mcp-http', url: 'https://mcp.linear.app/sse' })).toBe(
			'linear'
		);
		expect(
			suggestServiceName({ kind: 'openapi', spec: 'https://api.github.com/openapi.json' })
		).toBe('github');
		expect(
			suggestServiceName({
				kind: 'mcp-stdio',
				command: 'npx -y @modelcontextprotocol/server-github'
			})
		).toBe('github');
		expect(suggestServiceName({ kind: 'mcp-stdio', command: 'uvx mcp-server-fetch' })).toBe(
			'fetch'
		);
		expect(
			suggestServiceName({
				kind: 'mcp-stdio',
				command: 'npx -y @notionhq/notion-mcp-server'
			})
		).toBe('notion');
		expect(suggestNameFromUrl('https://WWW.Example.COM/mcp')).toBe('example');
		expect(suggestServiceName({ kind: 'mcp-stdio', command: 'npx My_Server' })).toBe('my-server');
	});
});
