<script lang="ts">
	import { icons, type IconName } from './icons.ts';

	let { name, class: className = 'size-5' }: { name: IconName; class?: string } = $props();

	const shapes = $derived(icons[name]);
</script>

<svg
	xmlns="http://www.w3.org/2000/svg"
	viewBox="0 0 24 24"
	fill="none"
	stroke="currentColor"
	stroke-width="2"
	stroke-linecap="round"
	stroke-linejoin="round"
	aria-hidden="true"
	class={className}
>
	{#each shapes as shape, index (index)}
		{#if shape.type === 'path'}
			<path d={shape.d} />
		{:else if shape.type === 'circle'}
			<circle cx={shape.cx} cy={shape.cy} r={shape.r} />
		{:else}
			<rect x={shape.x} y={shape.y} width={shape.width} height={shape.height} rx={shape.rx} />
		{/if}
	{/each}
</svg>
