<script lang="ts">
	import { onMount } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, ConfirmDialog, Skeleton, showToast } from '#lib/shared/ui/index.ts';
	import { clientName } from '#lib/shared/utils/client-names.ts';
	import { install, loadIntegrations, uninstall } from '../api.ts';
	import type { InstallReport, IntegrationView } from '../types.ts';

	let integrations = $state<IntegrationView[]>([]);
	let loading = $state(true);
	let error = $state('');
	let busyClient = $state<string | null>(null);
	let notes = $state<Record<string, string>>({});
	let disconnectTarget = $state<IntegrationView | null>(null);
	let disconnecting = $state(false);

	const disconnectName = $derived(disconnectTarget ? clientName(disconnectTarget.client) : '');

	onMount(() => {
		void load();
	});

	async function load(): Promise<void> {
		loading = true;
		error = '';

		try {
			integrations = await loadIntegrations();
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	function status(integration: IntegrationView): {
		label: string;
		dot: string;
		text: string;
	} {
		if (integration.mode === 'skill') {
			return {
				label: 'Connected with skill',
				dot: 'bg-emerald-500',
				text: 'text-zinc-600 dark:text-zinc-300'
			};
		}
		if (integration.mode === 'mcp') {
			return {
				label: 'Connected as MCP server',
				dot: 'bg-emerald-500',
				text: 'text-zinc-600 dark:text-zinc-300'
			};
		}
		if (integration.detected) {
			return {
				label: 'Not installed',
				dot: 'bg-zinc-400',
				text: 'text-zinc-500'
			};
		}
		return {
			label: 'Not found on this computer',
			dot: 'bg-zinc-300 dark:bg-zinc-600',
			text: 'text-zinc-400 dark:text-zinc-500'
		};
	}

	function applyReports(
		client: string,
		reports: InstallReport[],
		fallbackMode: 'skill' | 'mcp'
	): void {
		const report = reports.find((item) => item.client === client) ?? reports[0];
		const mode = report?.mode === 'skill' || report?.mode === 'mcp' ? report.mode : fallbackMode;

		integrations = integrations.map((integration) =>
			integration.client === client ? { ...integration, mode } : integration
		);

		if (report?.permission_note) {
			notes = { ...notes, [client]: report.permission_note };
		} else {
			const next = { ...notes };
			delete next[client];
			notes = next;
		}
	}

	async function connect(integration: IntegrationView, mcp = false): Promise<void> {
		if (busyClient) return;

		busyClient = integration.client;
		const name = clientName(integration.client);
		const fallbackMode = mcp || !integration.supports_skill ? 'mcp' : 'skill';

		try {
			const reports = await install(integration.client, mcp || !integration.supports_skill);
			applyReports(integration.client, reports, fallbackMode);
			showToast(`Restart ${name} to load Pandora.`);
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		} finally {
			busyClient = null;
		}
	}

	async function disconnect(): Promise<void> {
		const target = disconnectTarget;
		if (!target || disconnecting) return;

		disconnecting = true;
		const name = clientName(target.client);

		try {
			await uninstall(target.client);
			integrations = integrations.map((integration) =>
				integration.client === target.client ? { ...integration, mode: '' } : integration
			);
			const next = { ...notes };
			delete next[target.client];
			notes = next;
			disconnectTarget = null;
			showToast(`Disconnected ${name}.`);
		} catch (cause) {
			disconnectTarget = null;
			showToast(errorMessage(cause), 'error');
		} finally {
			disconnecting = false;
			busyClient = null;
		}
	}
</script>

<div>
	<h1 class="text-2xl font-semibold tracking-tight">AI clients</h1>
	<p class="mt-1 mb-6 text-sm text-zinc-500">Choose which AI apps can use your accounts.</p>

	{#if loading}
		<div class="grid gap-4 md:grid-cols-2">
			{#each [0, 1, 2, 3] as card (card)}
				<div
					class="rounded-xl border border-zinc-200 bg-white p-5 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
				>
					<Skeleton class="h-5 w-32" />
					<Skeleton class="mt-3 h-4 w-40" />
					<Skeleton class="mt-6 h-11 w-28 rounded-lg" />
				</div>
			{/each}
		</div>
	{:else if error}
		<div
			class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">Could not load AI clients</h2>
			<p class="mt-2 text-sm break-words text-zinc-500">{error}</p>
			<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void load()}>
				Try again
			</Button>
		</div>
	{:else if integrations.length === 0}
		<div
			class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			No AI clients found.
		</div>
	{:else}
		<div class="grid gap-4 md:grid-cols-2">
			{#each integrations as integration (integration.client)}
				{@const state = status(integration)}
				{@const name = clientName(integration.client)}
				{@const busy = busyClient !== null || disconnecting}
				<div
					class="flex flex-col rounded-xl border border-zinc-200 bg-white p-5 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
				>
					<h2 class="text-base font-semibold">{name}</h2>
					<p class="mt-1 flex items-center gap-2 text-sm {state.text}">
						<span class={['size-2 shrink-0 rounded-full', state.dot].join(' ')} aria-hidden="true"
						></span>
						{state.label}
					</p>

					{#if notes[integration.client]}
						<p
							class="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
						>
							{notes[integration.client]}
						</p>
					{/if}

					<div class="mt-4 flex flex-wrap items-center gap-x-4 gap-y-1">
						{#if integration.mode === ''}
							<Button
								variant={integration.detected ? 'primary' : 'secondary'}
								loading={busyClient === integration.client}
								disabled={busy}
								onclick={() => void connect(integration)}
							>
								{integration.detected ? 'Connect' : 'Connect anyway'}
							</Button>

							{#if integration.supports_skill}
								<button
									type="button"
									class="inline-flex min-h-11 items-center text-sm font-medium text-zinc-500 underline underline-offset-2 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 dark:text-zinc-400 dark:hover:text-zinc-100"
									disabled={busy}
									onclick={() => void connect(integration, true)}
								>
									Use MCP instead
								</button>
							{/if}
						{:else}
							<Button
								variant="secondary"
								disabled={busy}
								onclick={() => (disconnectTarget = integration)}
							>
								Disconnect
							</Button>

							{#if integration.supports_skill}
								<button
									type="button"
									class="inline-flex min-h-11 items-center text-sm font-medium text-zinc-500 underline underline-offset-2 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none disabled:cursor-not-allowed disabled:opacity-50 dark:text-zinc-400 dark:hover:text-zinc-100"
									disabled={busy}
									onclick={() => void connect(integration, integration.mode === 'skill')}
								>
									{integration.mode === 'skill' ? 'Switch to MCP' : 'Switch to skill'}
								</button>
							{/if}
						{/if}
					</div>
				</div>
			{/each}
		</div>

		<p class="mt-6 text-sm text-zinc-500">
			Skill: the AI runs the pandora command and reads a short guide. MCP: the AI talks to Pandora
			as an MCP server.
		</p>
	{/if}
</div>

{#if disconnectTarget}
	<ConfirmDialog
		title={`Disconnect ${disconnectName}`}
		message={`Disconnect ${disconnectName}? Pandora will remove its installation from this app.`}
		confirmLabel="Disconnect"
		loading={disconnecting}
		onclose={() => {
			if (!disconnecting) disconnectTarget = null;
		}}
		onconfirm={() => void disconnect()}
	/>
{/if}

<svelte:head>
	<title>AI clients · Pandora</title>
</svelte:head>
