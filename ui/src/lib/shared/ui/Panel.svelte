<script lang="ts">
	import { tick, type Snippet } from 'svelte';
	import Icon from './Icon.svelte';

	let {
		title,
		onclose,
		children,
		footer,
		body = $bindable()
	}: {
		title: string;
		onclose: () => void;
		children: Snippet;
		footer?: Snippet;
		body?: HTMLElement | null;
	} = $props();

	const titleId = $props.id();
	let closeButton = $state<HTMLButtonElement | null>(null);
	let panel = $state<HTMLDivElement | null>(null);

	$effect(() => {
		const previous = document.body.style.overflow;
		document.body.style.overflow = 'hidden';
		void tick().then(() => closeButton?.focus());

		return () => {
			document.body.style.overflow = previous;
		};
	});

	function trapFocus(event: KeyboardEvent): void {
		if (event.key !== 'Tab' || !panel) return;

		const focusable = Array.from(
			panel.querySelectorAll<HTMLElement>(
				'a[href], button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex]:not([tabindex="-1"])'
			)
		).filter((element) => element.offsetParent !== null);

		if (focusable.length === 0) return;

		const first = focusable[0];
		const last = focusable[focusable.length - 1];

		if (event.shiftKey && document.activeElement === first) {
			event.preventDefault();
			last.focus();
		} else if (!event.shiftKey && document.activeElement === last) {
			event.preventDefault();
			first.focus();
		}
	}
</script>

<svelte:window
	onkeydown={(event) => {
		if (event.key === 'Escape') onclose();
	}}
/>

<div class="fixed inset-0 z-50">
	<button
		type="button"
		tabindex="-1"
		class="absolute inset-0 h-full w-full cursor-default bg-zinc-950/50"
		aria-label="Close panel"
		onclick={onclose}
	></button>

	<div
		bind:this={panel}
		role="dialog"
		tabindex="-1"
		aria-modal="true"
		aria-labelledby={titleId}
		class="absolute inset-y-0 right-0 flex w-full flex-col bg-white shadow-2xl lg:w-[520px] dark:bg-zinc-900"
		onkeydown={trapFocus}
	>
		<header
			class="flex min-h-16 shrink-0 items-center justify-between gap-4 border-b border-zinc-200 px-4 sm:px-6 dark:border-zinc-800"
		>
			<h2 id={titleId} class="text-xl font-semibold">{title}</h2>
			<button
				bind:this={closeButton}
				type="button"
				class="-mr-2 inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
				aria-label="Close"
				onclick={onclose}
			>
				<Icon name="x" class="size-5" />
			</button>
		</header>

		<div bind:this={body} class="min-h-0 flex-1 overflow-y-auto px-4 py-5 sm:px-6">
			{@render children()}
		</div>

		{#if footer}
			<footer
				class="flex shrink-0 flex-wrap justify-end gap-3 border-t border-zinc-200 px-4 py-4 sm:px-6 dark:border-zinc-800"
			>
				{@render footer()}
			</footer>
		{/if}
	</div>
</div>
