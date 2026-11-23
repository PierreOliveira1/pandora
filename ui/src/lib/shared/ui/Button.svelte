<script lang="ts">
	import type { Snippet } from 'svelte';
	import type { HTMLButtonAttributes } from 'svelte/elements';
	import Icon from './Icon.svelte';
	import type { IconName } from './icons.ts';
	import Spinner from './Spinner.svelte';

	type Variant = 'primary' | 'secondary' | 'danger' | 'ghost';

	let {
		variant = 'primary',
		type = 'button',
		class: className = '',
		icon,
		loading = false,
		disabled = false,
		children,
		...rest
	}: HTMLButtonAttributes & {
		class?: string;
		variant?: Variant;
		icon?: IconName;
		loading?: boolean;
		children: Snippet;
	} = $props();

	const variants: Record<Variant, string> = {
		primary: 'bg-violet-600 text-white shadow-sm hover:bg-violet-700 disabled:hover:bg-violet-600',
		secondary:
			'border border-zinc-300 bg-white text-zinc-800 shadow-sm hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:text-zinc-100 dark:hover:bg-zinc-800',
		danger: 'bg-red-600 text-white shadow-sm hover:bg-red-700 disabled:hover:bg-red-600',
		ghost:
			'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
	};
</script>

<button
	{type}
	class={[
		'inline-flex min-h-11 items-center justify-center gap-2 rounded-lg px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-2 focus-visible:ring-offset-white disabled:cursor-not-allowed disabled:opacity-50 dark:focus-visible:ring-offset-zinc-950',
		variants[variant],
		className
	].join(' ')}
	disabled={disabled || loading}
	aria-busy={loading || undefined}
	{...rest}
>
	{#if loading}
		<Spinner />
	{:else if icon}
		<Icon name={icon} class="size-4 shrink-0" />
	{/if}
	{@render children()}
</button>
