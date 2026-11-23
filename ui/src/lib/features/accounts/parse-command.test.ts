import { describe, expect, it } from 'vitest';
import { parseCommandLine } from './parse-command.ts';

describe('command line parsing', () => {
	it('a command line is split into arguments respecting quotes', () => {
		expect(parseCommandLine('npx -y @modelcontextprotocol/server-github')).toEqual([
			'npx',
			'-y',
			'@modelcontextprotocol/server-github'
		]);
		expect(parseCommandLine(`tool "two words" 'three words'`)).toEqual([
			'tool',
			'two words',
			'three words'
		]);
		expect(parseCommandLine(`tool "" 'a b'`)).toEqual(['tool', '', 'a b']);
		expect(parseCommandLine(`  tool   --flag="a b"  `)).toEqual(['tool', '--flag=a b']);
	});
});
