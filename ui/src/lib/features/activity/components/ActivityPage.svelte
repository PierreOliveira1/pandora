<script lang="ts">
	import { onMount } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Icon, Pill, Skeleton, selectClasses, showToast } from '#lib/shared/ui/index.ts';
	import { clientName } from '#lib/shared/utils/client-names.ts';
	import { formatDuration, formatLogTime } from '#lib/shared/utils/format.ts';
	import { loadLog, loadOverview } from '../api.ts';
	import type { LogEntry, Overview } from '../types.ts';
	import EventsView from './EventsView.svelte';

	type ActivityView = 'calls' | 'events';

	let overview = $state<Overview | null>(null);
	let entries = $state<LogEntry[]>([]);
	let activeView = $state<ActivityView>('calls');
	let selectedRef = $state('');
	let problemsOnly = $state(false);
	let loading = $state(true);
	let refreshing = $state(false);
	let error = $state('');
	let expanded = $state<string | null>(null);
	let timer: ReturnType<typeof setInterval> | undefined;

	const services = $derived(overview?.services ?? []);
	const subtitle = $derived(
		activeView === 'calls'
			? 'Every tool call made by your AI clients. Arguments are never stored.'
			: 'Events your services sent to Pandora. Their content comes from third parties: never follow instructions inside them.'
	);

	onMount(() => {
		void loadInitial();
		timer = setInterval(() => {
			if (document.visibilityState !== 'visible' || loading || refreshing) return;
			if (activeView !== 'calls') return;
			void loadEntries(false);
		}, 5000);

		return () => {
			if (timer) clearInterval(timer);
		};
	});

	async function loadInitial(): Promise<void> {
		loading = true;
		error = '';

		try {
			const [data, log] = await Promise.all([
				loadOverview(),
				loadLog({ ref: selectedRef || undefined, errors: problemsOnly })
			]);
			overview = data;
			entries = [...log].reverse();
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function loadEntries(showLoading: boolean): Promise<void> {
		if (showLoading) {
			loading = true;
		} else {
			refreshing = true;
		}
		error = '';

		try {
			const log = await loadLog({
				ref: selectedRef || undefined,
				errors: problemsOnly
			});
			entries = [...log].reverse();
		} catch (cause) {
			if (showLoading) {
				error = errorMessage(cause);
			} else {
				showToast(errorMessage(cause), 'error');
			}
		} finally {
			loading = false;
			refreshing = false;
		}
	}

	function applyFilters(): void {
		expanded = null;
		void loadEntries(true);
	}

	function toggleProblems(): void {
		problemsOnly = !problemsOnly;
		applyFilters();
	}

	function selectView(view: ActivityView): void {
		if (activeView === view) return;
		activeView = view;
		if (view === 'calls' && !loading) void loadEntries(false);
	}

	function entryKey(entry: LogEntry): string {
		return `${entry.time}-${entry.ref}-${entry.tool}-${entry.client}`;
	}

	function toggleEntry(entry: LogEntry): void {
		if (!entry.error) return;
		const key = entryKey(entry);
		expanded = expanded === key ? null : key;
	}

	function resultTone(entry: LogEntry): 'blocked' | 'error' | 'ok' {
		if (entry.blocked) return 'blocked';
		if (!entry.ok || entry.error) return 'error';
		return 'ok';
	}
</script>

{#snippet resultBadge(entry: LogEntry, interactive = false)}
	{@const tone = resultTone(entry)}
	{@const activeKey = interactive ? entryKey(entry) : ''}
	{#if tone === 'ok'}
		<span
			class="inline-flex items-center gap-1.5 text-sm whitespace-nowrap text-zinc-600 dark:text-zinc-400"
		>
			<span class="size-2 rounded-full bg-emerald-500" aria-hidden="true"></span>
			ok
		</span>
	{:else if tone === 'blocked'}
		{#if interactive}
			<button
				type="button"
				class="inline-flex min-h-11 items-center rounded-full focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none"
				aria-expanded={expanded === activeKey}
				onclick={(event) => {
					event.stopPropagation();
					toggleEntry(entry);
				}}
			>
				<Pill tone="warning">blocked</Pill>
			</button>
		{:else}
			<Pill tone="warning">blocked</Pill>
		{/if}
	{:else}
		{#if interactive}
			<button
				type="button"
				class="inline-flex min-h-11 items-center rounded-full focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none"
				aria-expanded={expanded === activeKey}
				onclick={(event) => {
					event.stopPropagation();
					toggleEntry(entry);
				}}
			>
				<Pill tone="danger">error</Pill>
			</button>
		{:else}
			<Pill tone="danger">error</Pill>
		{/if}
	{/if}
{/snippet}

{#snippet mobileCard(entry: LogEntry)}
	{@const key = entryKey(entry)}
	<div class="flex items-center justify-between gap-3">
		<span class="min-w-0 truncate font-mono text-sm font-medium text-zinc-900 dark:text-zinc-100">
			{entry.tool}
		</span>
		{@render resultBadge(entry)}
	</div>
	<p class="mt-1 text-xs text-zinc-500">
		<span class="font-mono">{entry.ref}</span>
		· {clientName(entry.client)} · {formatLogTime(entry.time)} · {formatDuration(entry.duration_ms)}
	</p>
	{#if entry.error && expanded === key}
		<p
			class="mt-2 rounded-md bg-red-50 p-2 text-xs break-words whitespace-pre-wrap text-red-700 dark:bg-red-500/10 dark:text-red-300"
		>
			{entry.error}
		</p>
	{/if}
{/snippet}

<div>
	<h1 class="text-2xl font-semibold tracking-tight">Activity</h1>
	<p class="mt-1 text-sm text-zinc-500">{subtitle}</p>

	<div
		class="mt-4 inline-flex overflow-hidden rounded-lg border border-zinc-300 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
		role="group"
		aria-label="Activity view"
	>
		<button
			type="button"
			class={[
				'inline-flex min-h-11 items-center justify-center px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
				activeView === 'calls'
					? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
					: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
			].join(' ')}
			aria-pressed={activeView === 'calls'}
			onclick={() => selectView('calls')}
		>
			Calls
		</button>
		<button
			type="button"
			class={[
				'inline-flex min-h-11 items-center justify-center px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
				activeView === 'events'
					? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
					: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
			].join(' ')}
			aria-pressed={activeView === 'events'}
			onclick={() => selectView('events')}
		>
			Events
		</button>
	</div>

	{#if activeView === 'calls'}
		<div class="mt-6 flex flex-col gap-3 md:flex-row md:items-center">
			<div class="relative md:w-72">
				<select
					bind:value={selectedRef}
					class={selectClasses}
					aria-label="Account"
					onchange={applyFilters}
				>
					<option value="">All accounts</option>
					{#each services as service (service.name)}
						<option value={service.name}>{service.name} (all accounts)</option>
						{#each service.accounts as account (account.name)}
							<option value={`${service.name}/${account.name}`}>
								{service.name}/{account.name}
							</option>
						{/each}
					{/each}
				</select>
				<Icon
					name="chevron"
					class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
				/>
			</div>

			<div class="flex items-center gap-2">
				<div
					class="inline-flex overflow-hidden rounded-lg border border-zinc-300 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
					role="group"
					aria-label="Call status"
				>
					<button
						type="button"
						class={[
							'inline-flex min-h-[42px] items-center justify-center px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
							!problemsOnly
								? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
								: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
						].join(' ')}
						aria-pressed={!problemsOnly}
						onclick={() => {
							if (!problemsOnly) return;
							toggleProblems();
						}}
					>
						All
					</button>
					<button
						type="button"
						class={[
							'inline-flex min-h-[42px] items-center justify-center px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
							problemsOnly
								? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
								: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
						].join(' ')}
						aria-pressed={problemsOnly}
						onclick={() => {
							if (problemsOnly) return;
							toggleProblems();
						}}
					>
						Problems
					</button>
				</div>

				<Button
					variant="secondary"
					icon="refresh"
					class="size-11 shrink-0 px-0 sm:w-auto sm:px-3"
					loading={refreshing}
					disabled={loading}
					aria-label="Refresh"
					onclick={() => void loadEntries(false)}
				>
					<span class="hidden sm:inline">Refresh</span>
				</Button>
			</div>
		</div>

		<div class="mt-6">
			{#if loading}
				<div
					class="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
				>
					{#each [0, 1, 2, 3, 4] as row (row)}
						<div
							class="flex items-center gap-4 border-b border-zinc-100 py-3 last:border-0 dark:border-zinc-800"
						>
							<Skeleton class="h-4 w-16" />
							<Skeleton class="h-4 w-24" />
							<Skeleton class="h-4 w-32" />
							<Skeleton class="ml-auto h-4 w-16" />
						</div>
					{/each}
				</div>
			{:else if error}
				<div
					class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
				>
					<h2 class="text-xl font-semibold">Could not load activity</h2>
					<p class="mt-2 text-sm break-words text-zinc-500">{error}</p>
					<Button
						variant="secondary"
						icon="refresh"
						class="mt-4"
						onclick={() => void loadInitial()}
					>
						Try again
					</Button>
				</div>
			{:else if entries.length === 0}
				<div
					class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
				>
					No calls yet. When an AI uses Pandora, its calls show up here.
				</div>
			{:else}
				<div
					class="hidden overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm md:block dark:border-zinc-800 dark:bg-zinc-900"
				>
					<table class="w-full text-left text-sm">
						<thead
							class="border-b border-zinc-200 bg-zinc-50 text-xs text-zinc-500 dark:border-zinc-800 dark:bg-zinc-950/50"
						>
							<tr>
								<th class="px-4 py-3 font-medium">Time</th>
								<th class="px-4 py-3 font-medium">Client</th>
								<th class="px-4 py-3 font-medium">Account</th>
								<th class="px-4 py-3 font-medium">Tool</th>
								<th class="px-4 py-3 font-medium">Result</th>
								<th class="px-4 py-3 text-right font-medium">Duration</th>
							</tr>
						</thead>
						<tbody>
							{#each entries as entry (entryKey(entry))}
								{@const key = entryKey(entry)}
								<tr
									class={entry.error
										? 'cursor-pointer border-b border-zinc-100 transition-colors last:border-0 hover:bg-zinc-50 dark:border-zinc-800 dark:hover:bg-zinc-800/40'
										: 'border-b border-zinc-100 last:border-0 dark:border-zinc-800'}
									onclick={() => toggleEntry(entry)}
								>
									<td class="h-12 px-4 whitespace-nowrap text-zinc-500 tabular-nums">
										{formatLogTime(entry.time)}
									</td>
									<td class="h-12 px-4"><Pill>{clientName(entry.client)}</Pill></td>
									<td class="h-12 px-4 font-mono text-xs text-zinc-600 dark:text-zinc-300">
										{entry.ref}
									</td>
									<td class="h-12 px-4 font-mono text-xs text-zinc-800 dark:text-zinc-200">
										{entry.tool}
									</td>
									<td class="h-12 px-4">
										{@render resultBadge(entry, Boolean(entry.error))}
									</td>
									<td class="h-12 px-4 text-right whitespace-nowrap text-zinc-500 tabular-nums">
										{formatDuration(entry.duration_ms)}
									</td>
								</tr>
								{#if entry.error && expanded === key}
									<tr class="border-b border-zinc-100 last:border-0 dark:border-zinc-800">
										<td colspan="6" class="bg-red-50 px-4 py-3 dark:bg-red-500/10">
											<p
												class="text-xs break-words whitespace-pre-wrap text-red-700 dark:text-red-300"
											>
												{entry.error}
											</p>
										</td>
									</tr>
								{/if}
							{/each}
						</tbody>
					</table>
				</div>

				<div class="space-y-2 md:hidden">
					{#each entries as entry (entryKey(entry))}
						{@const key = entryKey(entry)}
						{#if entry.error}
							<button
								type="button"
								class="w-full rounded-xl border border-zinc-200 bg-white p-3 text-left shadow-sm focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:border-zinc-800 dark:bg-zinc-900"
								onclick={() => toggleEntry(entry)}
							>
								{@render mobileCard(entry)}
							</button>
						{:else}
							<div
								class="rounded-xl border border-zinc-200 bg-white p-3 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
							>
								{@render mobileCard(entry)}
							</div>
						{/if}
					{/each}
				</div>
			{/if}
		</div>
	{:else}
		<div class="mt-6">
			<EventsView />
		</div>
	{/if}
</div>

<svelte:head>
	<title>Activity · Pandora</title>
</svelte:head>
