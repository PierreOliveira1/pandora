const CLIENT_NAMES: Record<string, string> = {
	'claude-code': 'Claude Code',
	codex: 'Codex',
	opencode: 'OpenCode',
	cursor: 'Cursor',
	'claude-desktop': 'Claude Desktop',
	'pandora-ui': 'Pandora UI',
	terminal: 'Terminal'
};

export function clientName(client: string): string {
	const id = client.trim();
	if (!id) return '';

	if (id.startsWith('mcp:')) {
		const base = id.slice(4);
		return `${CLIENT_NAMES[base] ?? base} (MCP)`;
	}

	return CLIENT_NAMES[id] ?? id;
}
