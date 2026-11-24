import { parseCommandLine } from './parse-command.ts';
import type { ServiceKind } from './types.ts';

const GENERIC_HOST_LABELS = new Set(['mcp', 'api', 'ai', 'www', 'app']);
const PACKAGE_PREFIXES = ['server-', 'mcp-'];
const PACKAGE_SUFFIXES = ['-mcp-server', '-mcp', '-server'];

export type ServiceNameSource = {
	kind: ServiceKind | null;
	url?: string;
	spec?: string;
	apiBaseUrl?: string;
	command?: string;
};

export function normalizeServiceName(value: string): string {
	return value
		.toLowerCase()
		.replace(/[^a-z0-9-]+/g, '-')
		.replace(/-+/g, '-')
		.replace(/^-+/, '')
		.slice(0, 32)
		.replace(/-+$/, '');
}

export function suggestNameFromUrl(value: string): string {
	const trimmed = value.trim();
	if (!trimmed) return '';

	let parsed: URL;
	try {
		parsed = new URL(trimmed);
	} catch {
		return '';
	}

	const labels = parsed.hostname.toLowerCase().split('.').filter(Boolean);
	if (labels.length === 0) return '';
	if (labels.length > 1) labels.pop();

	const candidate = labels.find((label) => !GENERIC_HOST_LABELS.has(label)) ?? '';
	return normalizeServiceName(candidate);
}

export function suggestNameFromCommand(value: string): string {
	const args = parseCommandLine(value).filter((argument) => !argument.startsWith('-'));
	const packageName = args.at(-1)?.split('/').filter(Boolean).at(-1) ?? '';
	let candidate = packageName;

	let changed = true;
	while (changed) {
		changed = false;

		for (const prefix of PACKAGE_PREFIXES) {
			if (!candidate.startsWith(prefix)) continue;
			candidate = candidate.slice(prefix.length);
			changed = true;
		}

		for (const suffix of PACKAGE_SUFFIXES) {
			if (!candidate.endsWith(suffix)) continue;
			candidate = candidate.slice(0, -suffix.length);
			changed = true;
		}
	}

	return normalizeServiceName(candidate);
}

export function suggestServiceName(source: ServiceNameSource): string {
	if (source.kind === 'mcp-http' || source.kind === 'http') {
		return suggestNameFromUrl(source.url ?? '');
	}

	if (source.kind === 'openapi') {
		return suggestNameFromUrl(source.spec ?? '') || suggestNameFromUrl(source.apiBaseUrl ?? '');
	}

	if (source.kind === 'mcp-stdio') return suggestNameFromCommand(source.command ?? '');

	return '';
}
