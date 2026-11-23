export const session = $state({ expired: false });

export function markSessionExpired(): void {
	session.expired = true;
}
