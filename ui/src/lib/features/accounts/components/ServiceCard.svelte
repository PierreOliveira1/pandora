<script lang="ts">
	import { ActionMenu, Pill } from '#lib/shared/ui/index.ts';
	import { serviceKindLabel } from '../types.ts';
	import type { AccountView, ServiceView, StatusInfo, WebhookInfo } from '../types.ts';
	import AccountRow from './AccountRow.svelte';

	type MenuItem = {
		id: string;
		label: string;
		tone?: 'default' | 'danger';
		onselect: () => void;
	};

	let {
		service,
		statuses,
		statusLoading,
		loggingInRef,
		onRenameService,
		onRemoveService,
		onLogin,
		onRenameAccount,
		onRemoveAccount,
		onSetDefault,
		onLogout,
		onRules,
		webhooks,
		onRemoveWebhook
	}: {
		service: ServiceView;
		statuses: Record<string, StatusInfo>;
		statusLoading: boolean;
		loggingInRef: string | null;
		onRenameService: () => void;
		onRemoveService: () => void;
		onLogin: (account: AccountView) => void;
		onRenameAccount: (account: AccountView) => void;
		onRemoveAccount: (account: AccountView) => void;
		onSetDefault: (account: AccountView) => void;
		onLogout: (account: AccountView) => void;
		onRules: (account: AccountView) => void;
		webhooks: Record<string, WebhookInfo>;
		onRemoveWebhook: (account: AccountView) => void;
	} = $props();

	const toolsLabel = $derived(
		service.tools_known
			? `${service.tool_count} ${service.tool_count === 1 ? 'tool' : 'tools'}`
			: 'Tools not loaded'
	);

	const menuItems = $derived.by((): MenuItem[] => [
		{ id: 'rename', label: 'Rename service', onselect: onRenameService },
		{
			id: 'remove',
			label: 'Remove service',
			tone: 'danger',
			onselect: onRemoveService
		}
	]);
</script>

<section
	class="rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
>
	<div class="p-4 sm:p-5">
		<div class="flex items-center gap-2">
			<h2 class="min-w-0 truncate font-semibold">{service.name}</h2>
			<Pill>{serviceKindLabel(service.kind)}</Pill>
			<div class="ml-auto flex shrink-0 items-center gap-1">
				<span class="text-xs whitespace-nowrap text-zinc-500">{toolsLabel}</span>
				<div class="-my-2">
					<ActionMenu label={`Actions for ${service.name}`} items={menuItems} />
				</div>
			</div>
		</div>

		{#if service.description}
			<p class="mt-0.5 line-clamp-2 pr-12 text-sm text-zinc-500">{service.description}</p>
		{/if}
	</div>

	{#if service.accounts.length > 0}
		<ul>
			{#each service.accounts as account (account.name)}
				{@const ref = `${service.name}/${account.name}`}
				<li class="border-t border-zinc-200 dark:border-zinc-800">
					<AccountRow
						serviceName={service.name}
						{account}
						status={statuses[ref]}
						{statusLoading}
						auth={service.auth}
						loggingIn={loggingInRef === ref}
						onLogin={() => onLogin(account)}
						onRename={() => onRenameAccount(account)}
						onRemove={() => onRemoveAccount(account)}
						onSetDefault={() => onSetDefault(account)}
						onLogout={() => onLogout(account)}
						onRules={() => onRules(account)}
						webhook={webhooks[ref]}
						onRemoveWebhook={() => onRemoveWebhook(account)}
					/>
				</li>
			{/each}
		</ul>
	{:else}
		<p
			class="border-t border-zinc-200 px-4 py-4 text-sm text-zinc-500 sm:px-5 dark:border-zinc-800"
		>
			No accounts for this service.
		</p>
	{/if}
</section>
