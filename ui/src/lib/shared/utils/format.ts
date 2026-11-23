export function formatDuration(milliseconds: number): string {
	if (!Number.isFinite(milliseconds) || milliseconds <= 0) return '0 ms';
	if (milliseconds < 1000) return `${Math.round(milliseconds)} ms`;
	return `${(milliseconds / 1000).toFixed(1)} s`;
}

export function formatRelativeTime(value: string, now = Date.now()): string {
	const time = Date.parse(value);
	if (Number.isNaN(time)) return '';

	const elapsed = now - time;
	if (elapsed < 60_000) return 'just now';

	const minutes = Math.floor(elapsed / 60_000);
	if (minutes < 60) return `${minutes} min ago`;

	const hours = Math.floor(minutes / 60);
	if (hours < 24) return `${hours} h ago`;

	const days = Math.floor(hours / 24);
	if (days < 7) return `${days} d ago`;

	return new Intl.DateTimeFormat('en', { month: 'short', day: 'numeric' }).format(time);
}

export function formatLogTime(value: string, now = new Date()): string {
	const date = new Date(value);
	if (Number.isNaN(date.getTime())) return value;

	const time = new Intl.DateTimeFormat('en', {
		hour: '2-digit',
		minute: '2-digit',
		second: '2-digit',
		hour12: false
	}).format(date);

	const sameDay =
		date.getFullYear() === now.getFullYear() &&
		date.getMonth() === now.getMonth() &&
		date.getDate() === now.getDate();

	if (sameDay) return time;

	const options: Intl.DateTimeFormatOptions = { month: 'short', day: 'numeric' };
	if (date.getFullYear() !== now.getFullYear()) options.year = 'numeric';
	const day = new Intl.DateTimeFormat('en', options).format(date);
	return `${day}, ${time}`;
}
