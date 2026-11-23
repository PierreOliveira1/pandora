import type { JsonSchema, JsonValue } from '#lib/shared/api/types.ts';

export type ToolArgument = {
	name: string;
	type: string;
	required: boolean;
	description: string;
};

function argumentType(property: JsonSchema): string {
	const type = property.type;

	if (Array.isArray(type)) {
		const names = type.filter((entry) => entry !== 'null');
		return names.length > 0 ? names.join(' | ') : 'unknown';
	}

	if (type === 'array') {
		const items = property.items?.type;
		if (Array.isArray(items)) {
			return `array<${items.join(' | ')}>`;
		}
		return items ? `array<${items}>` : 'array';
	}

	return type ?? 'unknown';
}

export function listToolArguments(schema: JsonSchema | undefined | null): ToolArgument[] {
	if (!schema?.properties) return [];

	const required = new Set(schema.required ?? []);
	return Object.entries(schema.properties).map(([name, property]) => ({
		name,
		type: argumentType(property),
		required: required.has(name),
		description: property.description ?? ''
	}));
}

function emptyValue(property: JsonSchema): JsonValue {
	const type = Array.isArray(property.type)
		? property.type.find((entry) => entry !== 'null')
		: property.type;

	switch (type) {
		case 'number':
		case 'integer':
			return 0;
		case 'boolean':
			return false;
		case 'array':
			return [];
		case 'object':
			return {};
		default:
			return '';
	}
}

export function initialArguments(schema: JsonSchema | undefined | null): Record<string, JsonValue> {
	const args: Record<string, JsonValue> = {};
	if (!schema?.properties) return args;

	for (const name of schema.required ?? []) {
		const property = schema.properties[name];
		if (!property) continue;
		args[name] = emptyValue(property);
	}

	return args;
}
