export type IconName =
	| 'plus'
	| 'refresh'
	| 'check'
	| 'alert'
	| 'x'
	| 'menu'
	| 'chevron'
	| 'more'
	| 'login'
	| 'search'
	| 'copy';

export type IconShape =
	| { type: 'path'; d: string }
	| { type: 'circle'; cx: number; cy: number; r: number }
	| { type: 'rect'; x: number; y: number; width: number; height: number; rx?: number };

export const icons: Record<IconName, IconShape[]> = {
	plus: [
		{ type: 'path', d: 'M5 12h14' },
		{ type: 'path', d: 'M12 5v14' }
	],
	refresh: [
		{ type: 'path', d: 'M21 12a9 9 0 1 1-9-9c2.52 0 4.93 1 6.74 2.74L21 8' },
		{ type: 'path', d: 'M21 3v5h-5' }
	],
	check: [{ type: 'path', d: 'M20 6 9 17l-5-5' }],
	alert: [
		{ type: 'path', d: 'm21.73 18-8-14a2 2 0 0 0-3.46 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3' },
		{ type: 'path', d: 'M12 9v4' },
		{ type: 'path', d: 'M12 17h.01' }
	],
	x: [
		{ type: 'path', d: 'M18 6 6 18' },
		{ type: 'path', d: 'm6 6 12 12' }
	],
	menu: [
		{ type: 'path', d: 'M4 6h16' },
		{ type: 'path', d: 'M4 12h16' },
		{ type: 'path', d: 'M4 18h16' }
	],
	chevron: [{ type: 'path', d: 'm6 9 6 6 6-6' }],
	more: [
		{ type: 'circle', cx: 5, cy: 12, r: 1 },
		{ type: 'circle', cx: 12, cy: 12, r: 1 },
		{ type: 'circle', cx: 19, cy: 12, r: 1 }
	],
	login: [
		{ type: 'path', d: 'M15 3h4a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2h-4' },
		{ type: 'path', d: 'm10 17 5-5-5-5' },
		{ type: 'path', d: 'M15 12H3' }
	],
	search: [
		{ type: 'circle', cx: 11, cy: 11, r: 8 },
		{ type: 'path', d: 'm21 21-4.3-4.3' }
	],
	copy: [
		{ type: 'rect', x: 9, y: 9, width: 12, height: 12, rx: 2 },
		{ type: 'path', d: 'M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1' }
	]
};
