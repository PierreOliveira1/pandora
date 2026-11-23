<script lang="ts">
	import { Icon, Spinner } from '#lib/shared/ui/index.ts';
	import type { StatusInfo } from '../types.ts';

	let {
		accountRef,
		status,
		warning,
		loginWaiting,
		loginError
	}: {
		accountRef: string;
		status: StatusInfo;
		warning?: string;
		loginWaiting: boolean;
		loginError: string;
	} = $props();

	const connected = $derived(status.ok);
	const needsLogin = $derived(!status.ok && status.needs_login);
	const toolsLabel = $derived(`${status.tools} ${status.tools === 1 ? 'tool' : 'tools'} available`);
</script>

<div class="flex min-h-full flex-col items-center justify-center py-8 text-center">
	{#if connected}
		<span
			class="flex size-14 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400"
		>
			<Icon name="check" class="size-7" />
		</span>
		<h3 class="mt-5 text-xl font-semibold">Connected</h3>
		<p class="mt-2 text-sm text-zinc-500">{accountRef} · {toolsLabel}</p>
	{:else if needsLogin}
		<span
			class="flex size-14 items-center justify-center rounded-full bg-amber-50 text-amber-600 dark:bg-amber-500/15 dark:text-amber-400"
		>
			<Icon name="alert" class="size-7" />
		</span>
		<h3 class="mt-5 text-xl font-semibold">{accountRef} needs a login</h3>
		<p class="mt-2 max-w-sm text-sm text-zinc-500">
			We'll open your browser to sign in. If you're adding a second account of the same service,
			sign out of the first one in the browser or use a private window.
		</p>
		{#if loginWaiting}
			<div class="mt-6 flex items-center gap-2 text-sm text-zinc-500">
				<Spinner />
				<span>Waiting for you to finish in the browser...</span>
			</div>
		{/if}
	{:else}
		<span
			class="flex size-14 items-center justify-center rounded-full bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-400"
		>
			<Icon name="alert" class="size-7" />
		</span>
		<h3 class="mt-5 text-xl font-semibold">Saved, but we couldn't connect yet</h3>
		<p class="mt-2 max-w-sm text-sm break-words text-zinc-500">
			<span class="font-medium text-zinc-700 dark:text-zinc-300">{accountRef}</span>
			<span class="mt-1 block">{status.error ?? 'The connection failed.'}</span>
		</p>
	{/if}

	{#if warning}
		<div
			class="mt-5 w-full rounded-lg border border-amber-200 bg-amber-50 p-3 text-left text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
		>
			{warning}
		</div>
	{/if}

	{#if loginError}
		<div
			class="mt-5 w-full rounded-lg border border-red-200 bg-red-50 p-3 text-left text-sm break-words text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
		>
			{loginError}
		</div>
	{/if}
</div>
