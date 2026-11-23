<script lang="ts">
	let { class: className = 'h-4 w-full' }: { class?: string } = $props();
</script>

<div
	class={['animate-pulse rounded-md bg-zinc-200 dark:bg-zinc-800', className].join(' ')}
	aria-hidden="true"
></div>
