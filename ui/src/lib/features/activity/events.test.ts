import { describe, expect, it } from 'vitest';
import type { EventView } from './types.ts';
import { eventQuery, eventTopics, payloadPreview } from './events.ts';

function event(
	seq: number,
	service: string,
	account: string,
	topic: string,
	payload: EventView['payload']
): EventView {
	return {
		seq,
		id: `evt_${seq}`,
		received_at: '2026-11-24T12:00:00Z',
		service,
		account,
		topic,
		payload
	};
}

const events = [
	event(3, 'instagram', 'store', 'comments', { text: 'Hello' }),
	event(2, 'github', 'work', 'push', { ref: 'refs/heads/main' }),
	event(1, 'instagram', 'store', 'messaging', 'A direct message')
];

describe('events', () => {
	it('asks the server for the chosen service, account and topic', () => {
		expect(eventQuery('', '')).toEqual({});
		expect(eventQuery('instagram', '')).toEqual({ service: 'instagram' });
		expect(eventQuery('instagram/store', 'comments')).toEqual({
			service: 'instagram',
			account: 'store',
			topic: 'comments'
		});
	});

	it('shortens a long payload preview with an ellipsis', () => {
		const long = 'This comment is much longer than the preview can show. '.repeat(10);
		const preview = payloadPreview({ text: long }, 60);

		expect(preview.endsWith('…')).toBe(true);
		expect(preview.length).toBe(60);
		expect(payloadPreview('A direct message', 60)).toBe('A direct message');
	});

	it('lists each topic once from the loaded events', () => {
		expect(eventTopics(events)).toEqual(['comments', 'messaging', 'push']);
	});
});
