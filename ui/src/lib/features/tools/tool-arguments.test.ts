import { describe, expect, it } from 'vitest';
import { initialArguments, listToolArguments } from './tool-arguments.ts';
import type { JsonSchema } from './types.ts';

const schema: JsonSchema = {
	type: 'object',
	properties: {
		query: { type: 'string', description: 'Text to search for.' },
		limit: { type: 'integer', description: 'Maximum results.' },
		include_done: { type: 'boolean' },
		tags: { type: 'array', items: { type: 'string' } }
	},
	required: ['query', 'limit']
};

describe('tool arguments', () => {
	it("a tool's arguments are listed from its input schema with required ones marked", () => {
		expect(listToolArguments(schema)).toEqual([
			{
				name: 'query',
				type: 'string',
				required: true,
				description: 'Text to search for.'
			},
			{
				name: 'limit',
				type: 'integer',
				required: true,
				description: 'Maximum results.'
			},
			{
				name: 'include_done',
				type: 'boolean',
				required: false,
				description: ''
			},
			{
				name: 'tags',
				type: 'array<string>',
				required: false,
				description: ''
			}
		]);
	});

	it('the arguments editor starts with the required arguments of the right types', () => {
		expect(initialArguments(schema)).toEqual({
			query: '',
			limit: 0
		});
	});
});
