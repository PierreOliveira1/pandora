<script lang="ts">
	let { class: className = 'size-4' }: { class?: string } = $props();
</script>

<span
	class={[
		'inline-block shrink-0 animate-spin rounded-full border-2 border-current border-t-transparent',
		className
	].join(' ')}
	aria-hidden="true"
></span>
