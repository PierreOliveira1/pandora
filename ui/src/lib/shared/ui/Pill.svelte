<script lang="ts">
	import type { Snippet } from 'svelte';

	type Tone = 'neutral' | 'brand' | 'success' | 'warning' | 'danger';

	let {
		tone = 'neutral',
		children
	}: {
		tone?: Tone;
		children: Snippet;
	} = $props();

	const tones: Record<Tone, string> = {
		neutral: 'bg-zinc-100 text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300',
		brand: 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300',
		success: 'bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300',
		warning: 'bg-amber-50 text-amber-700 dark:bg-amber-500/15 dark:text-amber-300',
		danger: 'bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300'
	};
</script>

<span
	class={[
		'inline-flex items-center gap-1.5 rounded-full px-2 py-0.5 text-xs font-medium whitespace-nowrap',
		tones[tone]
	].join(' ')}
>
	{@render children()}
</span>
