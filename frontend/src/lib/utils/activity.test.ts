import { describe, expect, it } from 'vitest';
import { getActivityTimestamp } from './activity';

const date = (value: string) => new Date(value);

describe('getActivityTimestamp', () => {
	it('uses check-in time before update or creation time', () => {
		const createdAt = date('2026-01-01T00:00:00Z');
		const checkedInAt = date('2026-01-02T00:00:00Z');
		const updatedAt = date('2026-01-03T00:00:00Z');

		expect(getActivityTimestamp({ createdAt, checkedInAt, updatedAt })).toBe(checkedInAt);
	});

	it('uses updated time when the guest has not checked in', () => {
		const updatedAt = date('2026-01-03T00:00:00Z');
		expect(getActivityTimestamp({
			createdAt: date('2026-01-01T00:00:00Z'),
			updatedAt,
		})).toBe(updatedAt);
	});

	it('falls back to check-in time for legacy guests without updatedAt', () => {
		const checkedInAt = date('2026-01-02T00:00:00Z');
		expect(getActivityTimestamp({
			createdAt: date('2026-01-01T00:00:00Z'),
			checkedInAt,
		})).toBe(checkedInAt);
	});
});
