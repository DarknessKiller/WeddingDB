import { describe, it, expect, vi } from 'vitest';
import { canSaveGrants, grantsLoading, loadWeddingGrants } from './userGrants';

describe('wedding grants load', () => {
	it('keeps Save disabled with no granted ids when the load fails', async () => {
		const load = vi.fn().mockRejectedValue(new Error('Failed to load weddings'));
		const state = await loadWeddingGrants(load);

		expect(state).toEqual({ loading: false, error: 'Failed to load weddings', ids: [] });
		expect(canSaveGrants(state)).toBe(false);
	});

	it('allows Save only after the grants loaded', async () => {
		const pending = grantsLoading();
		expect(canSaveGrants(pending)).toBe(false);

		const state = await loadWeddingGrants(async () => [{ id: 'w1' }, { id: 'w2' }]);
		expect(state.ids).toEqual(['w1', 'w2']);
		expect(canSaveGrants(state)).toBe(true);
	});
});
