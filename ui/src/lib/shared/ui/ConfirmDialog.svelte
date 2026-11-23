<script lang="ts">
	import Button from './Button.svelte';
	import Dialog from './Dialog.svelte';

	let {
		title,
		message,
		confirmLabel = 'Remove',
		confirmVariant = 'danger',
		loading = false,
		onclose,
		onconfirm
	}: {
		title: string;
		message: string;
		confirmLabel?: string;
		confirmVariant?: 'primary' | 'secondary' | 'danger';
		loading?: boolean;
		onclose: () => void;
		onconfirm: () => void;
	} = $props();
</script>

<Dialog {title} {onclose}>
	<p class="text-sm break-words text-zinc-600 dark:text-zinc-300">{message}</p>

	{#snippet footer()}
		<Button variant="secondary" onclick={onclose} disabled={loading}>Cancel</Button>
		<Button variant={confirmVariant} {loading} onclick={onconfirm}>
			{confirmLabel}
		</Button>
	{/snippet}
</Dialog>
