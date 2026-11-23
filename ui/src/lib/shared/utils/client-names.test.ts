import { describe, expect, it } from 'vitest';
import { clientName } from './client-names.ts';

describe('client names', () => {
	it('client ids read as friendly names', () => {
		expect(clientName('claude-code')).toBe('Claude Code');
		expect(clientName('codex')).toBe('Codex');
		expect(clientName('opencode')).toBe('OpenCode');
		expect(clientName('mcp:cursor')).toBe('Cursor (MCP)');
		expect(clientName('pandora-ui')).toBe('Pandora UI');
		expect(clientName('terminal')).toBe('Terminal');
	});
});
