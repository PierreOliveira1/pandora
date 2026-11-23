export function parseCommandLine(input: string): string[] {
	const args: string[] = [];
	let current = '';
	let quote: '"' | "'" | null = null;
	let started = false;

	for (let index = 0; index < input.length; index += 1) {
		const char = input[index];

		if (quote) {
			if (char === quote) {
				quote = null;
				continue;
			}
			if (char === '\\' && quote === '"' && index + 1 < input.length) {
				index += 1;
				current += input[index];
				continue;
			}
			current += char;
			continue;
		}

		if (char === '"' || char === "'") {
			quote = char;
			started = true;
			continue;
		}

		if (char === '\\' && index + 1 < input.length) {
			index += 1;
			current += input[index];
			started = true;
			continue;
		}

		if (/\s/.test(char)) {
			if (started) {
				args.push(current);
				current = '';
				started = false;
			}
			continue;
		}

		current += char;
		started = true;
	}

	if (started) args.push(current);
	return args;
}
