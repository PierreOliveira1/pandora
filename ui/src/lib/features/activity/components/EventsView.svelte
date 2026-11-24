<script lang="ts">
	import { onMount } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Icon, Pill, Skeleton, selectClasses, showToast } from '#lib/shared/ui/index.ts';
	import type { JsonValue } from '#lib/shared/api/types.ts';
	import { formatLogTime } from '#lib/shared/utils/format.ts';
	import { loadEvents, loadWebhooks } from '../api.ts';
	import { eventQuery, eventTopics, payloadPreview } from '../events.ts';
	import type { EventView, WebhookInfo } from '../types.ts';

	type AccountGroup = {
		name: string;
		accounts: string[];
	};

	let events = $state<EventView[]>([]);
	let webhooks = $state<WebhookInfo[]>([]);
	let selectedAccount = $state('');
	let selectedTopic = $state('');
	let expanded = $state<number | null>(null);
	let loading = $state(true);
	let refreshing = $state(false);
	let error = $state('');
	let knownTopics = $state<string[]>([]);
	let request = 0;
	let timer: ReturnType<typeof setInterval> | undefined;

	const accountGroups = $derived.by((): AccountGroup[] => {
		const groups = new Map<string, Set<string>>();
		for (const webhook of webhooks) {
			const [service, account] = webhook.ref.split('/');
			if (!service || !account) continue;
			const accounts = groups.get(service) ?? new Set<string>();
			accounts.add(account);
			groups.set(service, accounts);
		}
		return [...groups]
			.sort(([left], [right]) => left.localeCompare(right))
			.map(([name, accounts]) => ({
				name,
				accounts: [...accounts].sort((left, right) => left.localeCompare(right))
			}));
	});

	const topics = $derived(
		[...new Set([...knownTopics, ...(selectedTopic ? [selectedTopic] : [])])].sort((left, right) =>
			left.localeCompare(right)
		)
	);
	const filtering = $derived(Boolean(selectedAccount || selectedTopic));

	onMount(() => {
		void loadInitial();
		timer = setInterval(() => {
			if (document.visibilityState !== 'visible' || loading || refreshing) return;
			void refresh();
		}, 5000);

		return () => {
			if (timer) clearInterval(timer);
		};
	});

	async function fetchAll(): Promise<{ hooks: WebhookInfo[]; entries: EventView[] } | null> {
		const current = ++request;
		const [hooks, entries] = await Promise.all([
			loadWebhooks(),
			loadEvents({ ...eventQuery(selectedAccount, selectedTopic), limit: 100 })
		]);
		if (current !== request) return null;
		return { hooks, entries };
	}

	function apply(result: { hooks: WebhookInfo[]; entries: EventView[] }): void {
		webhooks = result.hooks;
		events = result.entries;
		knownTopics = [...new Set([...knownTopics, ...eventTopics(result.entries)])];
	}

	async function loadInitial(): Promise<void> {
		loading = true;
		error = '';

		try {
			const result = await fetchAll();
			if (result) apply(result);
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	async function refresh(): Promise<void> {
		refreshing = true;

		try {
			const result = await fetchAll();
			if (result) apply(result);
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		} finally {
			refreshing = false;
		}
	}

	function applyFilters(): void {
		expanded = null;
		void refresh();
	}

	function prettyPayload(payload: JsonValue): string {
		return JSON.stringify(payload, null, 2) ?? '';
	}

	function toggleEvent(event: EventView): void {
		expanded = expanded === event.seq ? null : event.seq;
	}
</script>

{#if loading}
	<div
		class="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
	>
		{#each [0, 1, 2, 3] as row (row)}
			<div class="border-b border-zinc-100 py-3 last:border-0 dark:border-zinc-800">
				<div class="flex items-center gap-3">
					<Skeleton class="h-4 w-16" />
					<Skeleton class="h-4 w-28" />
					<Skeleton class="ml-auto h-4 w-12" />
				</div>
				<Skeleton class="mt-2 h-3 w-3/4" />
			</div>
		{/each}
	</div>
{:else if error}
	<div
		class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
	>
		<h2 class="text-xl font-semibold">Could not load events</h2>
		<p class="mt-2 text-sm break-words text-zinc-500">{error}</p>
		<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void loadInitial()}>
			Try again
		</Button>
	</div>
{:else if webhooks.length === 0}
	<div
		class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
	>
		<p>No webhooks yet.</p>
		<p class="mt-2 font-mono text-xs text-zinc-600 dark:text-zinc-400">
			<span class="whitespace-nowrap">pandora webhook add</span>
			<span class="whitespace-nowrap">&lt;service&gt;/&lt;account&gt;</span>
			<span class="whitespace-nowrap">--style hmac</span>
		</p>
	</div>
{:else}
	<div class="flex flex-col gap-3 md:flex-row md:items-center">
		<div class="relative md:w-72">
			<select
				bind:value={selectedAccount}
				class={selectClasses}
				aria-label="Account"
				onchange={applyFilters}
			>
				<option value="">All accounts</option>
				{#each accountGroups as group (group.name)}
					<option value={group.name}>{group.name} (all accounts)</option>
					{#each group.accounts as account (account)}
						<option value={`${group.name}/${account}`}>{group.name}/{account}</option>
					{/each}
				{/each}
			</select>
			<Icon
				name="chevron"
				class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
			/>
		</div>

		<div class="relative md:w-56">
			<select
				bind:value={selectedTopic}
				class={selectClasses}
				aria-label="Topic"
				onchange={applyFilters}
			>
				<option value="">All topics</option>
				{#each topics as topic (topic)}
					<option value={topic}>{topic}</option>
				{/each}
			</select>
			<Icon
				name="chevron"
				class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
			/>
		</div>
	</div>

	{#if events.length === 0 && !filtering}
		<div
			class="mt-4 rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			No events yet. They appear here as soon as the service sends one.
		</div>
	{:else if events.length === 0}
		<div
			class="mt-4 rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			No events match these filters.
		</div>
	{:else}
		<div
			class="mt-4 overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<ul>
				{#each events as event (event.seq)}
					<li class="border-b border-zinc-100 last:border-0 dark:border-zinc-800">
						<button
							type="button"
							class="flex min-h-11 w-full flex-col px-4 py-3 text-left transition-colors hover:bg-zinc-50 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset dark:hover:bg-zinc-800/40"
							aria-expanded={expanded === event.seq}
							aria-controls={`event-${event.seq}`}
							onclick={() => toggleEvent(event)}
						>
							<span class="flex flex-col gap-1 sm:flex-row sm:items-center sm:gap-3">
								<span class="flex min-w-0 items-center gap-2 sm:contents">
									<span class="shrink-0 text-sm text-zinc-500 tabular-nums">
										{formatLogTime(event.received_at)}
									</span>
									<span class="min-w-0 truncate font-mono text-xs text-zinc-600 dark:text-zinc-300">
										{event.service}/{event.account}
									</span>
								</span>
								<span class="flex items-center gap-2 sm:contents">
									{#if event.topic}
										<Pill>{event.topic}</Pill>
									{/if}
									<span class="ml-auto text-xs text-zinc-500 tabular-nums">#{event.seq}</span>
								</span>
							</span>
							<span class="mt-1.5 block truncate font-mono text-xs text-zinc-500">
								{payloadPreview(event.payload)}
							</span>
						</button>

						{#if expanded === event.seq}
							<div
								id={`event-${event.seq}`}
								class="border-t border-zinc-100 bg-zinc-50/50 px-4 py-3 dark:border-zinc-800 dark:bg-zinc-950/20"
							>
								<pre
									class="max-h-96 overflow-auto rounded-lg bg-zinc-50 p-3 font-mono text-xs break-words whitespace-pre-wrap text-zinc-700 dark:bg-zinc-800/50 dark:text-zinc-300">{prettyPayload(
										event.payload
									)}</pre>

								{#if event.headers && Object.keys(event.headers).length > 0}
									<div class="mt-3">
										<h3 class="text-xs font-medium text-zinc-500">Headers</h3>
										<dl class="mt-1 space-y-1 font-mono text-xs">
											{#each Object.entries(event.headers) as [name, value] (name)}
												<div class="flex min-w-0 flex-wrap gap-x-1.5">
													<dt class="text-zinc-500">{name}:</dt>
													<dd class="min-w-0 break-all text-zinc-600 dark:text-zinc-400">
														{value}
													</dd>
												</div>
											{/each}
										</dl>
									</div>
								{/if}
							</div>
						{/if}
					</li>
				{/each}
			</ul>
		</div>
	{/if}
{/if}
