import { describe, it, expect, vi } from 'vitest';

vi.mock('$lib/api/client', () => ({ apiFetch: vi.fn() }));

import { apiFetch } from '$lib/api/client';
import { updateKioskSettings } from './weddings';

describe('updateKioskSettings payload contract', () => {
	it('sends percentage positions verbatim and allows partial payloads', async () => {
		vi.mocked(apiFetch).mockResolvedValue({
			ok: true,
			json: async () => ({})
		} as Response);

		await updateKioskSettings('wid', {
			kioskBackgroundPosX: '37%',
			kioskBackgroundPosY: '80%'
		});

		expect(apiFetch).toHaveBeenCalledTimes(1);
		const [path, init] = vi.mocked(apiFetch).mock.calls[0];
		expect(path).toBe('/api/weddings/wid/kiosk');
		expect(init?.method).toBe('PUT');
		expect(JSON.parse(init?.body as string)).toEqual({
			kioskBackgroundPosX: '37%',
			kioskBackgroundPosY: '80%'
		});
	});
});
