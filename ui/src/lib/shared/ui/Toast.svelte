<script lang="ts">
	import Icon from './Icon.svelte';
	import { toasts } from './toasts.svelte.ts';
</script>

{#if toasts.length > 0}
	<div
		class="pointer-events-none fixed inset-x-4 bottom-4 z-[70] flex flex-col gap-2 sm:left-auto sm:w-96"
		aria-live="polite"
	>
		{#each toasts as toast (toast.id)}
			<div
				role={toast.tone === 'error' ? 'alert' : 'status'}
				class="pointer-events-auto flex items-start gap-3 rounded-xl border border-zinc-200 bg-white p-4 shadow-lg dark:border-zinc-800 dark:bg-zinc-900"
			>
				<span
					class={[
						'mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full',
						toast.tone === 'error'
							? 'bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-400'
							: 'bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400'
					].join(' ')}
				>
					<Icon name={toast.tone === 'error' ? 'alert' : 'check'} class="size-4" />
				</span>
				<p class="min-w-0 flex-1 text-sm text-zinc-700 dark:text-zinc-200">{toast.message}</p>
			</div>
		{/each}
	</div>
{/if}
