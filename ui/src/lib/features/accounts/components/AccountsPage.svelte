<script lang="ts">
	import { onMount } from 'svelte';
	import { ApiError, errorMessage } from '#lib/shared/api/client.ts';
	import { Button, ConfirmDialog, Skeleton, showToast } from '#lib/shared/ui/index.ts';
	import {
		loadOverview,
		loadStatus,
		loadWebhooks,
		logoutAccount,
		makeDefaultAccount,
		removeAccount,
		removeService,
		removeWebhook,
		renameAccount,
		renameService,
		startLogin,
		waitForLogin
	} from '../api.ts';
	import type { AccountView, Overview, ServiceView, StatusInfo, WebhookInfo } from '../types.ts';
	import AddAccountPanel from './AddAccountPanel.svelte';
	import RenameDialog from './RenameDialog.svelte';
	import RulesPanel from './RulesPanel.svelte';
	import ServiceCard from './ServiceCard.svelte';

	type RenameTarget =
		| { type: 'service'; service: ServiceView }
		| { type: 'account'; service: ServiceView; account: AccountView };

	type RemoveTarget = RenameTarget;
	type WebhookTarget = { service: ServiceView; account: AccountView };

	let overview = $state<Overview | null>(null);
	let loading = $state(true);
	let error = $state('');
	let statuses = $state<Record<string, StatusInfo>>({});
	let webhooks = $state<Record<string, WebhookInfo>>({});
	let statusLoading = $state(false);
	let statusUnavailable = $state(false);
	let addOpen = $state(false);
	let loggingInRef = $state<string | null>(null);
	let renameTarget = $state<RenameTarget | null>(null);
	let removeTarget = $state<RemoveTarget | null>(null);
	let removing = $state(false);
	let webhookRemoveTarget = $state<WebhookTarget | null>(null);
	let removingWebhook = $state(false);
	let rulesTarget = $state<string | null>(null);

	const removeMessage = $derived.by(() => {
		if (!removeTarget) return '';
		if (removeTarget.type === 'service') {
			return `Remove ${removeTarget.service.name}? All its accounts and saved credentials will be deleted.`;
		}
		return `Remove ${removeTarget.service.name}/${removeTarget.account.name}? Its saved credentials will be deleted.`;
	});

	const removeWebhookMessage = $derived.by(() => {
		if (!webhookRemoveTarget) return '';
		return `Remove the webhook of ${webhookRemoveTarget.service.name}/${webhookRemoveTarget.account.name}? Pandora stops receiving its events. Remove the callback URL from the service's settings too.`;
	});

	const renameTitle = $derived(
		renameTarget?.type === 'service' ? 'Rename service' : 'Rename account'
	);

	const renameInitialValue = $derived(
		renameTarget?.type === 'service'
			? renameTarget.service.name
			: (renameTarget?.account.name ?? '')
	);

	onMount(() => {
		void loadInitial();
	});

	async function loadInitial(): Promise<void> {
		loading = true;
		error = '';

		try {
			const [data, hooks] = await Promise.all([
				loadOverview(),
				loadWebhooks().catch((): WebhookInfo[] => [])
			]);
			overview = data;
			webhooks = Object.fromEntries(hooks.map((webhook) => [webhook.ref, webhook]));
			loading = false;
			void loadStatuses();
		} catch (cause) {
			error = errorMessage(cause);
			loading = false;
		}
	}

	async function loadStatuses(): Promise<void> {
		statusLoading = true;
		statusUnavailable = false;

		try {
			const list = await loadStatus();
			statuses = Object.fromEntries(list.map((status) => [status.ref, status]));
		} catch (cause) {
			statusUnavailable = true;
			if (cause instanceof ApiError && cause.status === 401) return;
		} finally {
			statusLoading = false;
		}
	}

	async function refreshAll(): Promise<void> {
		try {
			const [data, hooks] = await Promise.all([
				loadOverview(),
				loadWebhooks().catch((): WebhookInfo[] => [])
			]);
			overview = data;
			webhooks = Object.fromEntries(hooks.map((webhook) => [webhook.ref, webhook]));
			error = '';
			void loadStatuses();
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		}
	}

	function closeAddPanel(): void {
		addOpen = false;
		void refreshAll();
	}

	async function login(service: ServiceView, account: AccountView): Promise<void> {
		if (loggingInRef) return;

		const ref = `${service.name}/${account.name}`;
		loggingInRef = ref;

		try {
			const loginResult = await startLogin(service.name, account.name);
			await waitForLogin(loginResult.login_id);

			statuses = {
				...statuses,
				[ref]: {
					ref,
					ok: true,
					needs_login: false,
					tools: statuses[ref]?.tools ?? service.tool_count,
					latency_ms: 0
				}
			};
			showToast(`${ref} is connected.`);
			void loadStatuses();
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		} finally {
			loggingInRef = null;
		}
	}

	async function setDefault(service: ServiceView, account: AccountView): Promise<void> {
		const ref = `${service.name}/${account.name}`;

		try {
			await makeDefaultAccount(service.name, account.name);
			showToast(`${ref} is now the default account.`);
			await refreshAll();
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		}
	}

	async function logout(service: ServiceView, account: AccountView): Promise<void> {
		const ref = `${service.name}/${account.name}`;

		try {
			await logoutAccount(service.name, account.name);
			showToast(`Logged out of ${ref}.`);
			await refreshAll();
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
		}
	}

	async function saveRename(name: string): Promise<string | null> {
		const target = renameTarget;
		if (!target) return 'Nothing to rename.';

		try {
			if (target.type === 'service') {
				await renameService(target.service.name, name);
				showToast(`Renamed service ${target.service.name} to ${name}.`);
			} else {
				await renameAccount(target.service.name, target.account.name, name);
				showToast(`Renamed account ${target.service.name}/${target.account.name} to ${name}.`);
			}

			renameTarget = null;
			await refreshAll();
			return null;
		} catch (cause) {
			return errorMessage(cause);
		}
	}

	async function confirmRemove(): Promise<void> {
		const target = removeTarget;
		if (!target || removing) return;

		removing = true;

		try {
			if (target.type === 'service') {
				await removeService(target.service.name);
				showToast(`Removed service ${target.service.name}.`);
			} else {
				await removeAccount(target.service.name, target.account.name);
				showToast(`Removed account ${target.service.name}/${target.account.name}.`);
			}

			removeTarget = null;
			await refreshAll();
		} catch (cause) {
			removeTarget = null;
			showToast(errorMessage(cause), 'error');
		} finally {
			removing = false;
		}
	}

	async function confirmRemoveWebhook(): Promise<void> {
		const target = webhookRemoveTarget;
		if (!target || removingWebhook) return;

		removingWebhook = true;

		try {
			await removeWebhook(target.service.name, target.account.name);
			showToast('Webhook removed.');
			webhookRemoveTarget = null;
			await refreshAll();
		} catch (cause) {
			webhookRemoveTarget = null;
			showToast(errorMessage(cause), 'error');
		} finally {
			removingWebhook = false;
		}
	}
</script>

<div>
	<div class="mb-2 flex items-center justify-between gap-3">
		<h1 class="text-2xl font-semibold tracking-tight">Accounts</h1>
		<Button icon="plus" class="shrink-0" onclick={() => (addOpen = true)}>
			<span class="hidden sm:inline">Add account</span>
			<span class="sm:hidden">Add</span>
		</Button>
	</div>

	<p class="mb-6 text-sm text-zinc-500">
		Every account here is available to your AI clients through the pandora skill.
	</p>

	{#if loading}
		<div class="space-y-4">
			{#each [0, 1] as index (index)}
				<div
					class="rounded-xl border border-zinc-200 bg-white p-4 shadow-sm sm:p-5 dark:border-zinc-800 dark:bg-zinc-900"
				>
					<div class="flex items-start justify-between gap-3">
						<div class="flex items-center gap-2">
							<Skeleton class="h-5 w-32" />
							<Skeleton class="h-5 w-20 rounded-full" />
						</div>
						<Skeleton class="h-4 w-20" />
					</div>
					<Skeleton class="mt-3 h-4 w-64" />
					<div class="mt-5 space-y-4">
						{#each [0, 1] as row (row)}
							<div class="flex items-center justify-between gap-3">
								<div class="space-y-1.5">
									<Skeleton class="h-4 w-28" />
									<Skeleton class="h-3 w-48" />
								</div>
								<Skeleton class="h-4 w-24" />
							</div>
						{/each}
					</div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div
			class="rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">Could not load accounts</h2>
			<p class="mt-2 text-sm break-words text-zinc-500">{error}</p>
			<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void loadInitial()}>
				Try again
			</Button>
		</div>
	{:else if overview && overview.services.length === 0}
		<div
			class="rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">No accounts yet</h2>
			<p class="mx-auto mt-2 max-w-md text-sm text-zinc-500">
				Add a remote MCP, a local MCP, an OpenAPI spec or your own commands.
			</p>
			<Button icon="plus" class="mt-5" onclick={() => (addOpen = true)}>Add account</Button>
		</div>
	{:else if overview}
		{#if statusUnavailable}
			<div
				class="mb-4 flex flex-wrap items-center justify-between gap-3 rounded-xl border border-amber-200 bg-amber-50 px-4 py-2 text-sm text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
			>
				<span>Could not check account statuses.</span>
				<Button
					variant="ghost"
					icon="refresh"
					class="px-2 text-amber-800 hover:bg-amber-100 dark:text-amber-300 dark:hover:bg-amber-500/10"
					onclick={() => void loadStatuses()}
				>
					Check again
				</Button>
			</div>
		{/if}

		<div class="space-y-4">
			{#each overview.services as service (service.name)}
				<ServiceCard
					{service}
					{statuses}
					{statusLoading}
					{loggingInRef}
					onRenameService={() => (renameTarget = { type: 'service', service })}
					onRemoveService={() => (removeTarget = { type: 'service', service })}
					onLogin={(account) => void login(service, account)}
					onRenameAccount={(account) => (renameTarget = { type: 'account', service, account })}
					onRemoveAccount={(account) => (removeTarget = { type: 'account', service, account })}
					onSetDefault={(account) => void setDefault(service, account)}
					onLogout={(account) => void logout(service, account)}
					onRules={(account) => (rulesTarget = `${service.name}/${account.name}`)}
					{webhooks}
					onRemoveWebhook={(account) => (webhookRemoveTarget = { service, account })}
				/>
			{/each}
		</div>
	{/if}
</div>

{#if addOpen}
	<AddAccountPanel services={overview?.services ?? []} onclose={closeAddPanel} />
{/if}

{#if renameTarget}
	<RenameDialog
		title={renameTitle}
		initialValue={renameInitialValue}
		onclose={() => (renameTarget = null)}
		onsave={saveRename}
	/>
{/if}

{#if removeTarget}
	<ConfirmDialog
		title={removeTarget.type === 'service' ? 'Remove service' : 'Remove account'}
		message={removeMessage}
		loading={removing}
		onclose={() => {
			if (!removing) removeTarget = null;
		}}
		onconfirm={() => void confirmRemove()}
	/>
{/if}

{#if webhookRemoveTarget}
	<ConfirmDialog
		title="Remove webhook"
		message={removeWebhookMessage}
		loading={removingWebhook}
		onclose={() => {
			if (!removingWebhook) webhookRemoveTarget = null;
		}}
		onconfirm={() => void confirmRemoveWebhook()}
	/>
{/if}

{#if rulesTarget}
	<RulesPanel
		accountRef={rulesTarget}
		onclose={() => {
			rulesTarget = null;
			void refreshAll();
		}}
	/>
{/if}

<svelte:head>
	<title>Accounts · Pandora</title>
</svelte:head>
