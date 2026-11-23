<script lang="ts">
	import { tick } from 'svelte';
	import Icon from './Icon.svelte';

	type MenuItem = {
		id: string;
		label: string;
		tone?: 'default' | 'danger';
		onselect: () => void;
	};

	let {
		label,
		items,
		align = 'end'
	}: {
		label: string;
		items: MenuItem[];
		align?: 'start' | 'end';
	} = $props();

	const menuId = $props.id();
	let open = $state(false);
	let trigger = $state<HTMLButtonElement | null>(null);
	let panel = $state<HTMLDivElement | null>(null);

	async function show(): Promise<void> {
		open = true;
		await tick();
		panel?.querySelector<HTMLButtonElement>('[role="menuitem"]')?.focus();
	}

	async function hide(refocus = true): Promise<void> {
		if (!open) return;
		open = false;
		if (refocus) {
			await tick();
			trigger?.focus();
		}
	}

	function toggle(): void {
		if (open) {
			void hide();
			return;
		}
		void show();
	}

	function choose(item: MenuItem): void {
		void hide();
		item.onselect();
	}

	function handlePointerDown(event: PointerEvent): void {
		if (!open) return;
		const target = event.target as Node | null;
		if (!target) return;
		if (trigger?.contains(target) || panel?.contains(target)) return;
		void hide(false);
	}

	function handleKeydown(event: KeyboardEvent): void {
		if (!open || event.key !== 'Escape') return;
		event.preventDefault();
		void hide();
	}
</script>

<svelte:window onpointerdown={handlePointerDown} onkeydown={handleKeydown} />

<div class="relative inline-flex">
	<button
		bind:this={trigger}
		type="button"
		class="inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-zinc-500 transition-colors hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
		aria-label={label}
		aria-haspopup="menu"
		aria-expanded={open}
		aria-controls={menuId}
		onclick={toggle}
	>
		<Icon name="more" class="size-5" />
	</button>

	{#if open}
		<div
			bind:this={panel}
			id={menuId}
			role="menu"
			class={[
				'absolute top-full z-30 mt-1 min-w-48 rounded-lg border border-zinc-200 bg-white p-1 shadow-lg dark:border-zinc-700 dark:bg-zinc-900',
				align === 'end' ? 'right-0' : 'left-0'
			].join(' ')}
		>
			{#each items as item (item.id)}
				<button
					type="button"
					role="menuitem"
					class={[
						'flex min-h-11 w-full items-center rounded-md px-3 text-left text-sm transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none',
						item.tone === 'danger'
							? 'text-red-600 hover:bg-red-50 dark:text-red-400 dark:hover:bg-red-500/10'
							: 'text-zinc-700 hover:bg-zinc-100 dark:text-zinc-200 dark:hover:bg-zinc-800'
					].join(' ')}
					onclick={() => choose(item)}
				>
					{item.label}
				</button>
			{/each}
		</div>
	{/if}
</div>
