import { describe, expect, it } from 'vitest';
import { validateName } from './schemas.ts';

describe('account schemas', () => {
	it('names accept only lowercase letters, digits and dashes', () => {
		expect(validateName('todoist')).toBeNull();
		expect(validateName('billing-api-2')).toBeNull();
		expect(validateName('a'.repeat(32))).toBeNull();

		expect(validateName('Todoist')).not.toBeNull();
		expect(validateName('todoist_api')).not.toBeNull();
		expect(validateName('-todoist')).not.toBeNull();
		expect(validateName('a'.repeat(33))).not.toBeNull();
		expect(validateName('')).not.toBeNull();
	});
});
