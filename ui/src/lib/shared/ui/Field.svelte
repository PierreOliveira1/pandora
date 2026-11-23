<script lang="ts">
	import type { Snippet } from 'svelte';

	let {
		label,
		id,
		hint,
		error,
		children
	}: {
		label: string;
		id: string;
		hint?: string;
		error?: string;
		children: Snippet;
	} = $props();
</script>

<div class="min-w-0">
	<label for={id} class="mb-1.5 block text-sm font-medium text-zinc-700 dark:text-zinc-300">
		{label}
	</label>
	{@render children()}
	{#if error}
		<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{error}</p>
	{:else if hint}
		<p class="mt-1.5 text-xs text-zinc-500">{hint}</p>
	{/if}
</div>
