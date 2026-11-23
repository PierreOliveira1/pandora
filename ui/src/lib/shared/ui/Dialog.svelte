<script lang="ts">
	import type { Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		title,
		onclose,
		children,
		footer
	}: {
		title: string;
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
	} = $props();

	const titleId = $props.id();
	let dialog = $state<HTMLDialogElement | null>(null);

	$effect(() => {
		dialog?.showModal();
	});

	function handleBackdropClick(event: MouseEvent): void {
		if (event.target === dialog) onclose();
	}
</script>

<dialog
	bind:this={dialog}
	aria-labelledby={titleId}
	class="m-auto w-[calc(100%-2rem)] max-w-md rounded-xl border border-zinc-200 bg-white p-0 text-zinc-900 shadow-xl backdrop:bg-zinc-950/50 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-100"
	oncancel={(event) => {
		event.preventDefault();
		onclose();
	}}
	{onclose}
	onclick={handleBackdropClick}
>
	<div class="flex items-start justify-between gap-4 px-5 pt-5">
		<h2 id={titleId} class="text-xl font-semibold">{title}</h2>
		<button
			type="button"
			class="-mr-2 inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
			aria-label="Close"
			onclick={onclose}
		>
			<Icon name="x" class="size-5" />
		</button>
	</div>

	<div class="px-5 py-4">
		{@render children()}
	</div>

	{#if footer}
		<div
			class="flex flex-wrap justify-end gap-3 border-t border-zinc-200 px-5 py-4 dark:border-zinc-800"
		>
			{@render footer()}
		</div>
	{/if}
</dialog>
