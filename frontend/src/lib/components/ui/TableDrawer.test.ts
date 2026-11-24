import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import TableDrawer from './TableDrawer.svelte';
import { clampTableCapacity } from '#lib/utils/seat.ts';

describe('TableDrawer capacity', () => {
	it('cannot go below the highest occupied seat', () => {
		// Summed pax (5) is lower than the highest seat end (8): the server floor wins.
		const { body } = render(TableDrawer, {
			props: { title: 'Edit Table', name: 'Table 1', capacity: 2, occupied: 5, seatedFloor: 8, startEditing: true, onSave: () => {}, onClose: () => {} }
		});

		expect(body).toContain('min="8"');
		expect(body).toContain('max="200"');
		expect(body).toContain('Capacity cannot be below the 8 guests already seated.');
		// HTML min is only advisory: Save itself must refuse this state.
		expect(body).toMatch(/<button[^>]*disabled[^>]*>[^<]*<svg[\s\S]*?Save/);
	});

	it('clamps an out-of-range capacity to the backend cap', () => {
		// The max attribute alone does not stop typing 999: Save must refuse it,
		// and committing the field clamps it.
		const { body } = render(TableDrawer, {
			props: { title: 'Edit Table', name: 'Table 1', capacity: 999, seatedFloor: 8, startEditing: true, onSave: () => {}, onClose: () => {} }
		});
		expect(body).toContain('max="200"');
		expect(body).toContain('Capacity cannot exceed 200.');
		expect(body).toMatch(/<button[^>]*disabled[^>]*>[^<]*<svg[\s\S]*?Save/);
		expect(clampTableCapacity(999, 8)).toBe(200);
		expect(clampTableCapacity(2, 8)).toBe(8);
	});
});
