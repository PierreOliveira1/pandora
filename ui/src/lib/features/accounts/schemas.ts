export const NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/;

export function validateName(value: string): string | null {
	const name = value.trim();
	if (!name) return 'Enter a name.';
	if (!NAME_PATTERN.test(name)) {
		return 'Use lowercase letters, digits and dashes, up to 32 characters.';
	}
	return null;
}

export function validateUrl(value: string): string | null {
	const url = value.trim();
	if (!url) return 'Enter a URL.';

	let parsed: URL;
	try {
		parsed = new URL(url);
	} catch {
		return 'Enter a valid URL, like https://mcp.example.com/mcp.';
	}

	if (parsed.protocol !== 'https:' && parsed.protocol !== 'http:') {
		return 'Use an http:// or https:// URL.';
	}
	return null;
}

export function validateSpec(value: string): string | null {
	const spec = value.trim();
	if (!spec) return 'Enter the spec URL or file path.';
	if (spec.includes('://')) return validateUrl(spec);
	return null;
}
