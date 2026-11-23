<script lang="ts">
	import { onMount } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import {
		Button,
		Icon,
		Pill,
		Skeleton,
		fieldClasses,
		selectClasses
	} from '#lib/shared/ui/index.ts';
	import { formatRelativeTime } from '#lib/shared/utils/format.ts';
	import { loadOverview, loadTools } from '../api.ts';
	import type { Overview, ServiceView, ToolCatalog, ToolInfo } from '../types.ts';
	import AddCommandToolPanel from './AddCommandToolPanel.svelte';
	import ToolPanel from './ToolPanel.svelte';

	let { onOpenRules }: { onOpenRules: (ref: string) => void } = $props();

	let overview = $state<Overview | null>(null);
	let selectedServiceName = $state('');
	let catalog = $state<ToolCatalog | null>(null);
	let search = $state('');
	let loading = $state(true);
	let pageError = $state('');
	let catalogLoading = $state(false);
	let catalogError = $state('');
	let refreshing = $state(false);
	let selectedTool = $state<ToolInfo | null>(null);
	let addOpen = $state(false);

	const services = $derived(overview?.services ?? []);
	const selectedService = $derived(
		services.find((service) => service.name === selectedServiceName) ?? services[0]
	);
	const filteredTools = $derived.by(() => {
		const tools = catalog?.tools ?? [];
		const query = search.trim().toLowerCase();
		if (!query) return tools;

		return tools.filter(
			(tool) =>
				tool.name.toLowerCase().includes(query) ||
				(tool.description ?? '').toLowerCase().includes(query)
		);
	});
	const toolCount = $derived(catalog?.tools.length ?? 0);
	const updated = $derived(catalog ? formatRelativeTime(catalog.fetched_at) : '');

	onMount(() => {
		void loadPage();
	});

	async function loadPage(): Promise<void> {
		loading = true;
		pageError = '';

		try {
			overview = await loadOverview();
			const nextName = services.some((service) => service.name === selectedServiceName)
				? selectedServiceName
				: (services[0]?.name ?? '');
			selectedServiceName = nextName;
			loading = false;

			if (nextName) {
				catalog = null;
				await loadCatalog(false);
			}
		} catch (cause) {
			pageError = errorMessage(cause);
			loading = false;
		}
	}

	async function loadCatalog(refresh = false, silent = false): Promise<void> {
		const serviceName = selectedServiceName;
		if (!serviceName) {
			catalog = null;
			return;
		}

		if (silent) {
			refreshing = true;
		} else {
			catalogLoading = true;
		}
		catalogError = '';

		try {
			const data = await loadTools(serviceName, refresh);
			if (selectedServiceName === serviceName) catalog = data;
		} catch (cause) {
			if (selectedServiceName === serviceName) catalogError = errorMessage(cause);
		} finally {
			if (selectedServiceName === serviceName) {
				refreshing = false;
				catalogLoading = false;
			}
		}
	}

	function changeService(): void {
		search = '';
		catalog = null;
		catalogError = '';
		void loadCatalog(false);
	}

	function toolTone(tool: ToolInfo): { label: string; tone: 'success' | 'warning' | 'danger' } {
		if (tool.annotations?.readOnlyHint) return { label: 'read-only', tone: 'success' };
		if (tool.annotations?.destructiveHint) return { label: 'destructive', tone: 'danger' };
		return { label: 'changes data', tone: 'warning' };
	}

	function addCreated(): void {
		addOpen = false;
		void loadCatalog(true, true);
	}

	function toolRemoved(): void {
		selectedTool = null;
		void loadCatalog(true, true);
	}

	const toolsLabel = $derived(`${toolCount} ${toolCount === 1 ? 'tool' : 'tools'}`);
</script>

<div>
	<h1 class="text-2xl font-semibold tracking-tight">Tools</h1>
	<p class="mt-1 mb-6 text-sm text-zinc-500">What your AI can do through each account.</p>

	{#if loading}
		<div class="space-y-4">
			<Skeleton class="h-11 w-full rounded-lg" />
			<div
				class="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				{#each [0, 1, 2, 3] as row (row)}
					<div class="border-b border-zinc-100 py-3 last:border-0 dark:border-zinc-800">
						<Skeleton class="h-4 w-48" />
						<Skeleton class="mt-2 h-3 w-full max-w-md" />
					</div>
				{/each}
			</div>
		</div>
	{:else if pageError}
		<div
			class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">Could not load tools</h2>
			<p class="mt-2 text-sm break-words text-zinc-500">{pageError}</p>
			<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void loadPage()}>
				Try again
			</Button>
		</div>
	{:else if services.length === 0}
		<div
			class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">No services yet</h2>
			<p class="mx-auto mt-2 max-w-md text-sm text-zinc-500">
				Add an account from Accounts to see the tools it exposes.
			</p>
			<a
				href="/"
				class="mt-5 inline-flex min-h-11 items-center justify-center rounded-lg bg-violet-600 px-4 text-sm font-medium text-white hover:bg-violet-700 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-2 focus-visible:outline-none dark:focus-visible:ring-offset-zinc-950"
			>
				Go to Accounts
			</a>
		</div>
	{:else}
		<div class="flex flex-col gap-3 sm:flex-row sm:items-center">
			<div class="relative sm:w-56">
				<select
					bind:value={selectedServiceName}
					class={selectClasses}
					aria-label="Service"
					onchange={changeService}
				>
					{#each services as service (service.name)}
						<option value={service.name}>{service.name}</option>
					{/each}
				</select>
				<Icon
					name="chevron"
					class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
				/>
			</div>

			<div class="flex min-w-0 flex-1 items-center gap-2">
				<div class="relative min-w-0 flex-1">
					<Icon
						name="search"
						class="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-zinc-400"
					/>
					<input
						bind:value={search}
						type="search"
						class={`${fieldClasses} pl-9`}
						placeholder="Search tools..."
						aria-label="Search tools"
						autocomplete="off"
					/>
				</div>
				<Button
					variant="secondary"
					icon="refresh"
					class="size-11 shrink-0 px-0 sm:w-auto sm:px-3"
					loading={refreshing}
					disabled={catalogLoading}
					aria-label="Refresh"
					onclick={() => void loadCatalog(true, true)}
				>
					<span class="hidden sm:inline">Refresh</span>
				</Button>
			</div>
		</div>

		<div class="mt-4 mb-4 flex flex-wrap items-center justify-between gap-3">
			<p class="text-sm text-zinc-500">
				{#if catalogLoading}
					Loading tools...
				{:else if catalog}
					{toolsLabel} · updated {updated || 'just now'}
				{/if}
			</p>

			{#if selectedService?.kind === 'command'}
				<Button variant="secondary" icon="plus" onclick={() => (addOpen = true)}>
					Add command tool
				</Button>
			{/if}
		</div>

		{#if catalog?.warning}
			<div
				class="mb-4 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
			>
				{catalog.warning}
			</div>
		{/if}

		{#if catalogLoading}
			<div
				class="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				{#each [0, 1, 2, 3] as row (row)}
					<div class="border-b border-zinc-100 py-3 last:border-0 dark:border-zinc-800">
						<Skeleton class="h-4 w-48" />
						<Skeleton class="mt-2 h-3 w-full max-w-md" />
					</div>
				{/each}
			</div>
		{:else if catalogError}
			<div
				class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				<h2 class="text-lg font-semibold">Could not load tools</h2>
				<p class="mt-2 text-sm break-words text-zinc-500">{catalogError}</p>
				<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void loadCatalog()}>
					Try again
				</Button>
			</div>
		{:else if filteredTools.length === 0}
			<div
				class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				{search.trim() ? 'No tools match your search.' : 'No tools found for this service.'}
			</div>
		{:else}
			<div
				class="overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
			>
				<ul>
					{#each filteredTools as tool (tool.name)}
						{@const status = toolTone(tool)}
						<li class="border-b border-zinc-200 last:border-0 dark:border-zinc-800">
							<button
								type="button"
								class="flex min-h-11 w-full flex-col items-start gap-1 px-4 py-3 text-left transition-colors hover:bg-zinc-50 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset sm:px-5 dark:hover:bg-zinc-800/50"
								onclick={() => (selectedTool = tool)}
							>
								<span class="flex flex-wrap items-center gap-2">
									<span class="font-mono text-sm font-medium text-zinc-900 dark:text-zinc-100">
										{tool.name}
									</span>
									<Pill tone={status.tone}>{status.label}</Pill>
								</span>
								{#if tool.description}
									<span class="line-clamp-2 text-sm text-zinc-500 sm:line-clamp-1">
										{tool.description}
									</span>
								{/if}
							</button>
						</li>
					{/each}
				</ul>
			</div>
		{/if}
	{/if}
</div>

{#if selectedTool && selectedService}
	<ToolPanel
		service={selectedService}
		tool={selectedTool}
		onclose={() => (selectedTool = null)}
		onremoved={toolRemoved}
		oneditrules={onOpenRules}
	/>
{/if}

{#if addOpen && selectedService}
	<AddCommandToolPanel
		service={selectedService}
		onclose={() => (addOpen = false)}
		oncreated={addCreated}
	/>
{/if}

<svelte:head>
	<title>Tools · Pandora</title>
</svelte:head>
