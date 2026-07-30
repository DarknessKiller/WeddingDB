import { apiFetch } from './client';

export async function exportAngpaoReport(weddingId: string, format: 'csv' | 'xlsx'): Promise<void> {
	const res = await apiFetch(`/api/weddings/${weddingId}/reports/angpao?format=${format}`);
	if (!res.ok) {
		const err = await res.text().catch(() => 'Export failed');
		throw new Error(err || 'Export failed');
	}
	const blob = await res.blob();
	const disposition = res.headers.get('Content-Disposition');
	const filename = disposition?.match(/filename="(.+?)"/)?.[1] ?? `angpao-report.${format}`;
	const url = URL.createObjectURL(blob);
	const a = document.createElement('a');
	a.href = url;
	a.download = filename;
	a.click();
	URL.revokeObjectURL(url);
}
