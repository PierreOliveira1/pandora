<script lang="ts">
	let {
		checked = $bindable(false),
		label,
		description,
		disabled = false
	}: {
		checked?: boolean;
		label: string;
		description?: string;
		disabled?: boolean;
	} = $props();
</script>

<label
	class={[
		'flex min-h-11 items-center justify-between gap-4',
		disabled ? 'cursor-not-allowed opacity-50' : 'cursor-pointer'
	].join(' ')}
>
	<span class="min-w-0">
		<span class="block text-sm font-medium text-zinc-700 dark:text-zinc-300">{label}</span>
		{#if description}
			<span class="mt-0.5 block text-xs text-zinc-500">{description}</span>
		{/if}
	</span>
	<input type="checkbox" role="switch" bind:checked {disabled} class="peer sr-only" />
	<span
		class={[
			'relative h-6 w-11 shrink-0 rounded-full transition-colors peer-focus-visible:ring-2 peer-focus-visible:ring-violet-500 peer-focus-visible:ring-offset-2 peer-focus-visible:outline-none dark:peer-focus-visible:ring-offset-zinc-900',
			checked ? 'bg-zinc-900 dark:bg-zinc-100' : 'bg-zinc-300 dark:bg-zinc-700'
		].join(' ')}
		aria-hidden="true"
	>
		<span
			class={[
				'absolute top-0.5 left-0.5 size-5 rounded-full bg-white shadow transition-transform',
				checked ? 'translate-x-5' : ''
			].join(' ')}
		></span>
	</span>
</label>
