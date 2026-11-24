import type {
	CommandParam,
	CommandParamType,
	CommandToolInput,
	HttpRequestSpec,
	HttpToolMethod,
	ServiceView
} from './types.ts';

export type HttpKeyValue = {
	key: string;
	value: string;
};

export type HttpBodyMode = 'none' | 'json' | 'form';

export type HttpParameterInput = {
	name: string;
	type: CommandParamType;
	required: boolean;
	description: string;
};

export type HttpToolForm = {
	name: string;
	description: string;
	url: string;
	method: HttpToolMethod;
	path: string;
	query: HttpKeyValue[];
	headers: HttpKeyValue[];
	bodyMode: HttpBodyMode;
	body: HttpKeyValue[];
	parameters: HttpParameterInput[];
	readonly: boolean;
	timeoutSec: number;
};

export type FormattedToolText = {
	json: boolean;
	formatted: string;
};

export function keyValuesToRecord(rows: HttpKeyValue[]): Record<string, string> | undefined {
	const values: Record<string, string> = {};

	for (const row of rows) {
		const key = row.key.trim();
		if (key) values[key] = row.value;
	}

	return Object.keys(values).length > 0 ? values : undefined;
}

export function findSecretEnv(values: string[], secretNames: string[]): string | null {
	const secrets = new Set(secretNames);
	const pattern = /\{\{\s*env\.([A-Za-z0-9_]+)\s*\}\}/g;

	for (const value of values) {
		pattern.lastIndex = 0;
		let match = pattern.exec(value);
		while (match) {
			if (match[1] && secrets.has(match[1])) return match[1];
			match = pattern.exec(value);
		}
	}

	return null;
}

export function findMissingParameters(values: string[], parameterNames: string[]): string[] {
	const declared = new Set(parameterNames);
	const missing = new Set<string>();
	const pattern = /\{\{\s*([A-Za-z0-9_-]+)\s*\}\}/g;

	for (const value of values) {
		pattern.lastIndex = 0;
		let match = pattern.exec(value);
		while (match) {
			if (match[1] && !declared.has(match[1])) missing.add(match[1]);
			match = pattern.exec(value);
		}
	}

	return [...missing];
}

export function buildHttpToolInput(form: HttpToolForm): CommandToolInput {
	const bodyDisabled = form.method === 'GET' || form.method === 'HEAD';
	const http: HttpRequestSpec = {
		method: form.method,
		path: form.path.trim()
	};

	if (form.url) http.url = form.url;

	const query = keyValuesToRecord(form.query);
	if (query) http.query = query;

	const headers = keyValuesToRecord(form.headers);
	if (headers) http.headers = headers;

	const body = keyValuesToRecord(form.body);
	if (!bodyDisabled && form.bodyMode === 'form' && body) http.form = body;
	if (!bodyDisabled && form.bodyMode === 'json' && body) http.json = body;

	const params: Record<string, CommandParam> = {};
	for (const parameter of form.parameters) {
		const name = parameter.name.trim();
		if (!name) continue;

		const param: CommandParam = { type: parameter.type };
		if (parameter.description.trim()) param.description = parameter.description.trim();
		if (parameter.required) param.required = true;
		params[name] = param;
	}

	return {
		name: form.name.trim(),
		description: form.description.trim() || undefined,
		http,
		params,
		readonly: bodyDisabled || form.readonly,
		timeout_sec: form.timeoutSec
	};
}

export function requestUrl(
	service: ServiceView,
	spec: Pick<HttpRequestSpec, 'url' | 'path'>
): string {
	const urlNames = Object.keys(service.urls ?? {});
	const selected = spec.url || (urlNames.length === 1 ? urlNames[0] : '');
	const base = selected ? service.urls?.[selected] : undefined;
	const path = spec.path ?? '';

	if (!base) return path;
	if (!path) return base.replace(/\/$/, '');

	return `${base.replace(/\/$/, '')}/${path.replace(/^\//, '')}`;
}

export function requestPreview(
	service: ServiceView,
	spec: Pick<HttpRequestSpec, 'url' | 'method' | 'path' | 'query'>
): string {
	const url = requestUrl(service, spec);
	const query = Object.entries(spec.query ?? {})
		.filter(([key]) => key.trim())
		.map(([key, value]) => `${key}=${value}`)
		.join('&');
	const separator = url.includes('?') ? '&' : '?';

	return `${spec.method} ${url}${query ? `${separator}${query}` : ''}`;
}

export function formatToolText(text: string): FormattedToolText {
	const trimmed = text.trim();
	if (!trimmed) return { json: false, formatted: text };

	try {
		const parsed = JSON.parse(trimmed);
		if (parsed !== null && typeof parsed === 'object') {
			return { json: true, formatted: JSON.stringify(parsed, null, 2) };
		}
	} catch {
		return { json: false, formatted: text };
	}

	return { json: false, formatted: text };
}
