import { OpenPanel } from '@openpanel/web';
import { browser } from '$app/environment';
import { env } from '$env/dynamic/public';

let op: OpenPanel | undefined;

function client(): OpenPanel | undefined {
	if (!browser || !env.PUBLIC_OPENPANEL_CLIENT_ID) return;

	op ??= new OpenPanel({
		clientId: env.PUBLIC_OPENPANEL_CLIENT_ID,
		trackScreenViews: true,
		trackOutgoingLinks: true,
		trackAttributes: true
	});

	return op;
}

export function track(
	event: string,
	properties?: Record<string, string | number | boolean | null>
) {
	client()?.track(event, properties);
}

export function identifyUser(email: string, name?: string, role?: string) {
	const normalizedEmail = email.trim().toLowerCase();
	if (!normalizedEmail) return;

	client()?.identify({
		profileId: normalizedEmail,
		email: normalizedEmail,
		firstName: name || undefined,
		...(role ? { properties: { role } } : {})
	});
}

export function resetAnalytics() {
	client()?.clear?.();
}
