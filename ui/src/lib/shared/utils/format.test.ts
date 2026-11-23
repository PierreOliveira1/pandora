import { describe, expect, it } from 'vitest';
import { formatDuration } from './format.ts';

describe('format', () => {
	it('durations read as milliseconds or seconds', () => {
		expect(formatDuration(12)).toBe('12 ms');
		expect(formatDuration(999)).toBe('999 ms');
		expect(formatDuration(1400)).toBe('1.4 s');
	});
});
