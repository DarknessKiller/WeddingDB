import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import EditToolbar from './EditToolbar.svelte';

describe('EditToolbar table capacity', () => {
	it('cannot go below the highest occupied seat', () => {
		// Guests at seats 1 and 8: the server floor is 8, the summed pax is 2.
		const { body } = render(EditToolbar, {
			props: {
				hallWidth: 860,
				hallHeight: 1000,
				selectedId: 't1',
				isTableSelected: true,
				selectedItem: { kind: 'table', id: 't1', capacity: 10 },
				seatedPax: 8,
				onSave: () => {},
				onCancel: () => {},
				onDelete: () => {},
				onAddElement: () => {},
				onUpdateSelected: () => {},
				onWidthChange: () => {},
				onHeightChange: () => {}
			}
		});

		expect(body).toContain('min="8"');
		expect(body).toContain('max="200"');
	});
});
