/**
 * Load state for a user's wedding grants. The endpoint replaces every grant, so
 * an empty id list is only safe to save after the load succeeded.
 */
export type GrantsState = {
	loading: boolean;
	error: string | null;
	ids: string[];
};

export function grantsLoading(): GrantsState {
	return { loading: true, error: null, ids: [] };
}

/** Loads a user's wedding grants; a failure keeps Save disabled instead of meaning "none". */
export async function loadWeddingGrants(load: () => Promise<{ id: string }[]>): Promise<GrantsState> {
	try {
		const weds = await load();
		return { loading: false, error: null, ids: weds.map((w) => w.id) };
	} catch (e) {
		return { loading: false, error: e instanceof Error ? e.message : 'Failed to load weddings', ids: [] };
	}
}

export function canSaveGrants(state: GrantsState): boolean {
	return !state.loading && state.error === null;
}
