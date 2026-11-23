import { describe, expect, it } from 'vitest';
import { render } from 'svelte/server';
import EditToolbar from './EditToolbar.svelte';

describe('EditToolbar table capacity', () => {
	it('cannot go below the guests already seated', () => {
		const { body } = render(EditToolbar, {
			props: {
				hallWidth: 860,
				hallHeight: 1000,
				selectedId: 't1',
				isTableSelected: true,
				selectedItem: { kind: 'table', id: 't1', capacity: 10 },
				seatedPax: 7,
				onSave: () => {},
				onCancel: () => {},
				onDelete: () => {},
				onAddElement: () => {},
				onUpdateSelected: () => {},
				onWidthChange: () => {},
				onHeightChange: () => {}
			}
		});

		expect(body).toContain('min="7"');
	});
});
