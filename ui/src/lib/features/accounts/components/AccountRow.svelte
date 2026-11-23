<script lang="ts">
	import { ActionMenu, Button, Icon, Pill, Spinner } from '#lib/shared/ui/index.ts';
	import type { AccountView, AuthMode, StatusInfo } from '../types.ts';

	type MenuItem = {
		id: string;
		label: string;
		tone?: 'default' | 'danger';
		onselect: () => void;
	};

	let {
		serviceName,
		account,
		status,
		statusLoading,
		auth,
		loggingIn,
		onLogin,
		onRename,
		onRemove,
		onSetDefault,
		onLogout,
		onRules
	}: {
		serviceName: string;
		account: AccountView;
		status: StatusInfo | undefined;
		statusLoading: boolean;
		auth: AuthMode;
		loggingIn: boolean;
		onLogin: () => void;
		onRename: () => void;
		onRemove: () => void;
		onSetDefault: () => void;
		onLogout: () => void;
		onRules: () => void;
	} = $props();

	let expanded = $state(false);

	const ref = $derived(`${serviceName}/${account.name}`);
	const hasError = $derived(Boolean(status?.error));
	const needsLogin = $derived(Boolean(status?.needs_login) && !status?.ok);

	const menuItems = $derived.by((): MenuItem[] => {
		const items: MenuItem[] = [];

		if (!account.default) {
			items.push({ id: 'default', label: 'Set as default', onselect: onSetDefault });
		}
		items.push({ id: 'rules', label: 'Rules', onselect: onRules });
		items.push({ id: 'rename', label: 'Rename', onselect: onRename });
		if (auth === 'oauth') {
			items.push({ id: 'logout', label: 'Log out', onselect: onLogout });
		}
		items.push({ id: 'remove', label: 'Remove', tone: 'danger', onselect: onRemove });

		return items;
	});

	function toggleError(): void {
		if (hasError) expanded = !expanded;
	}

	function statusTitle(): string | undefined {
		return status?.error;
	}
</script>

{#snippet accountInfo()}
	<div class="flex flex-wrap items-center gap-2">
		<span class="font-medium">{account.name}</span>
		{#if account.default}
			<Pill tone="brand">default</Pill>
		{/if}
		{#if account.readonly}
			<Pill tone="warning">read-only</Pill>
		{/if}
		{#if account.allow.length > 0 || account.deny.length > 0}
			<Pill>rules</Pill>
		{/if}
	</div>
	{#if account.description}
		<p class="mt-0.5 text-sm text-zinc-500">{account.description}</p>
	{/if}
{/snippet}

<div
	class={[
		'px-4 py-3 sm:px-5',
		hasError ? 'transition-colors hover:bg-zinc-50 dark:hover:bg-zinc-800/40' : ''
	].join(' ')}
>
	<div class="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between">
		{#if hasError}
			<button
				type="button"
				class="min-h-11 min-w-0 rounded-md text-left focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none sm:flex-1"
				aria-expanded={expanded}
				onclick={toggleError}
			>
				{@render accountInfo()}
			</button>
		{:else}
			<div class="min-w-0 sm:flex-1">
				{@render accountInfo()}
			</div>
		{/if}

		<div class="flex w-full items-center justify-between gap-2 sm:w-auto sm:justify-end">
			<div class="flex min-w-0 flex-wrap items-center gap-2">
				{#if statusLoading && !status}
					<Pill tone="neutral">
						<Spinner class="size-3" />
						Checking
					</Pill>
				{:else if status?.ok}
					<span
						class="inline-flex items-center gap-1.5 text-sm whitespace-nowrap text-zinc-600 dark:text-zinc-400"
					>
						<span class="size-2 rounded-full bg-emerald-500" aria-hidden="true"></span>
						Connected
					</span>
				{:else if needsLogin}
					<Pill tone="warning">Needs login</Pill>
					<Button
						variant="secondary"
						class="px-3"
						loading={loggingIn}
						disabled={loggingIn}
						onclick={(event) => {
							event.stopPropagation();
							onLogin();
						}}
					>
						{loggingIn ? 'Waiting for browser...' : 'Log in'}
					</Button>
				{:else if hasError}
					<button
						type="button"
						class="inline-flex min-h-11 items-center gap-1.5 text-sm whitespace-nowrap text-red-600 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-red-400"
						title={statusTitle()}
						aria-expanded={expanded}
						onclick={(event) => {
							event.stopPropagation();
							toggleError();
						}}
					>
						<span class="size-2 rounded-full bg-red-500" aria-hidden="true"></span>
						Error
						<Icon
							name="chevron"
							class={['size-4 transition-transform', expanded ? 'rotate-180' : ''].join(' ')}
						/>
					</button>
				{:else}
					<span
						class="inline-flex items-center gap-1.5 text-sm whitespace-nowrap text-zinc-500 dark:text-zinc-400"
					>
						<span class="size-2 rounded-full bg-zinc-400" aria-hidden="true"></span>
						Status unavailable
					</span>
				{/if}
			</div>

			<ActionMenu label={`Actions for ${ref}`} items={menuItems} />
		</div>
	</div>

	{#if hasError && expanded}
		<div
			class="mt-3 rounded-lg border border-red-200 bg-red-50 p-3 text-sm break-words whitespace-pre-wrap text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
		>
			{status?.error}
		</div>
	{/if}
</div>
