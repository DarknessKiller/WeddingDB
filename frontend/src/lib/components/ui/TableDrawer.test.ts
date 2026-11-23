import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import TableDrawer from './TableDrawer.svelte';

describe('TableDrawer capacity', () => {
	it('cannot go below the guests already seated', () => {
		const { body } = render(TableDrawer, {
			props: { title: 'Edit Table', name: 'Table 1', capacity: 2, occupied: 5, startEditing: true, onSave: () => {}, onClose: () => {} }
		});

		expect(body).toContain('min="5"');
		expect(body).toContain('Capacity cannot be below the 5 guests already seated.');
		// HTML min is only advisory: Save itself must refuse this state.
		expect(body).toMatch(/<button[^>]*disabled[^>]*>[^<]*<svg[\s\S]*?Save/);
	});
});
