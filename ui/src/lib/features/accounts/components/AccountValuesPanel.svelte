<script lang="ts">
	import { untrack } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Panel, Pill, fieldClasses, showToast } from '#lib/shared/ui/index.ts';
	import { setAccountEnv, unsetAccountEnv } from '../api.ts';
	import type { AccountView } from '../types.ts';

	let {
		serviceName,
		account,
		onclose,
		onchanged
	}: {
		serviceName: string;
		account: AccountView;
		onclose: () => void;
		onchanged: () => Promise<void>;
	} = $props();

	let env = $state(untrack(() => ({ ...(account.env ?? {}) })));
	let secretEnv = $state(untrack(() => [...(account.secret_env ?? [])]));
	let name = $state('');
	let value = $state('');
	let saving = $state(false);
	let removingKey = $state<string | null>(null);
	let error = $state('');

	const accountRef = $derived(`${serviceName}/${account.name}`);
	const secretSet = $derived(new Set(secretEnv));
	const values = $derived(
		Object.entries(env)
			.filter(([key]) => !secretSet.has(key))
			.sort(([left], [right]) => left.localeCompare(right))
	);
	const secrets = $derived([...secretEnv].sort());

	$effect(() => {
		env = { ...(account.env ?? {}) };
		secretEnv = [...(account.secret_env ?? [])];
	});

	async function save(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (saving) return;

		error = '';
		const key = name.trim();

		if (!key) {
			error = 'Enter a name.';
			return;
		}

		saving = true;

		try {
			await setAccountEnv(serviceName, account.name, key, value);
			env = { ...env, [key]: value };
			showToast(`Value saved for ${accountRef}.`);
			name = '';
			value = '';
			await onchanged();
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			saving = false;
		}
	}

	async function remove(key: string): Promise<void> {
		if (removingKey) return;

		error = '';
		removingKey = key;

		try {
			await unsetAccountEnv(serviceName, account.name, key);
			env = Object.fromEntries(Object.entries(env).filter(([entry]) => entry !== key));
			showToast(`Removed ${key}.`);
			await onchanged();
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			removingKey = null;
		}
	}
</script>

<Panel title={`Values · ${accountRef}`} {onclose}>
	<div class="space-y-6">
		<section>
			<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Values</h3>

			{#if values.length > 0}
				<ul class="mt-2 divide-y divide-zinc-200 dark:divide-zinc-800">
					{#each values as [key, entryValue] (key)}
						<li class="flex min-h-11 items-center gap-3 py-2">
							<span class="shrink-0 font-mono text-xs font-medium">{key}</span>
							<span
								class="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500"
								title={entryValue}
							>
								{entryValue}
							</span>
							<Button
								variant="ghost"
								class="min-h-11 shrink-0 px-3"
								loading={removingKey === key}
								disabled={Boolean(removingKey)}
								aria-label={`Remove ${key}`}
								onclick={() => void remove(key)}
							>
								Remove
							</Button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="mt-2 text-sm text-zinc-500">No values yet.</p>
			{/if}
		</section>

		{#if secrets.length > 0}
			<section>
				<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Secrets</h3>
				<ul class="mt-2 divide-y divide-zinc-200 dark:divide-zinc-800">
					{#each secrets as key (key)}
						<li class="flex min-h-11 items-center gap-2 py-2">
							<span class="font-mono text-xs font-medium">{key}</span>
							<Pill>secret</Pill>
						</li>
					{/each}
				</ul>
				<p class="mt-2 text-xs text-zinc-500">
					Secrets are kept in the keychain. To change one, remove the account and add it again.
				</p>
			</section>
		{/if}

		<section>
			<form class="grid gap-2 sm:grid-cols-[8rem_1fr_auto]" onsubmit={save}>
				<input
					bind:value={name}
					type="text"
					class={fieldClasses}
					placeholder="STORE_ID"
					aria-label="Name"
					autocomplete="off"
					spellcheck="false"
				/>
				<input
					bind:value
					type="text"
					class={fieldClasses}
					placeholder="Value"
					aria-label="Value"
					autocomplete="off"
					spellcheck="false"
				/>
				<Button type="submit" class="w-full sm:w-auto" loading={saving}>Save</Button>
			</form>
			{#if error}
				<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{error}</p>
			{/if}
			<p class="mt-1.5 text-xs text-zinc-500">
				Use <code class="font-mono">{'{{env.NAME}}'}</code> in a tool's path, query or body.
			</p>
		</section>
	</div>
</Panel>
