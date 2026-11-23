<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import {
		Button,
		Icon,
		Panel,
		Skeleton,
		Switch,
		fieldClasses,
		showToast
	} from '#lib/shared/ui/index.ts';
	import { loadPolicy, savePolicy } from '../api.ts';

	let { accountRef, onclose }: { accountRef: string; onclose: () => void } = $props();

	const { serviceName, accountName } = untrack(() => {
		const separator = accountRef.indexOf('/');
		return {
			serviceName: separator === -1 ? accountRef : accountRef.slice(0, separator),
			accountName: separator === -1 ? '' : accountRef.slice(separator + 1)
		};
	});

	let readonly = $state(false);
	let allow = $state<string[]>([]);
	let deny = $state<string[]>([]);
	let allowInput = $state('');
	let denyInput = $state('');
	let loading = $state(true);
	let loadError = $state('');
	let saving = $state(false);
	let apiError = $state('');
	let panelBody = $state<HTMLElement | null>(null);

	onMount(() => {
		void load();
	});

	async function load(): Promise<void> {
		loading = true;
		loadError = '';

		try {
			const policy = await loadPolicy(serviceName, accountName);
			readonly = policy.readonly;
			allow = [...policy.allow];
			deny = [...policy.deny];
		} catch (cause) {
			loadError = errorMessage(cause);
		} finally {
			loading = false;
		}
	}

	function addDeny(): void {
		const pattern = denyInput.trim();
		if (!pattern) return;
		if (!deny.includes(pattern)) deny = [...deny, pattern];
		denyInput = '';
	}

	function addAllow(): void {
		const pattern = allowInput.trim();
		if (!pattern) return;
		if (!allow.includes(pattern)) allow = [...allow, pattern];
		allowInput = '';
	}

	async function save(): Promise<void> {
		if (saving) return;

		apiError = '';
		saving = true;

		try {
			const policy = await savePolicy(serviceName, accountName, {
				readonly,
				allow,
				deny
			});
			readonly = policy.readonly;
			allow = [...policy.allow];
			deny = [...policy.deny];
			showToast(`Rules saved for ${accountRef}.`);
			onclose();
		} catch (cause) {
			apiError = errorMessage(cause);
			await tick();
			panelBody?.scrollTo({ top: panelBody.scrollHeight, behavior: 'smooth' });
		} finally {
			saving = false;
		}
	}
</script>

<Panel title={`Rules · ${accountRef}`} {onclose} bind:body={panelBody}>
	{#if loading}
		<div class="space-y-4">
			<Skeleton class="h-16 w-full rounded-lg" />
			<Skeleton class="h-32 w-full rounded-lg" />
			<Skeleton class="h-32 w-full rounded-lg" />
		</div>
	{:else if loadError}
		<div
			class="rounded-lg border border-red-200 bg-red-50 p-4 text-sm text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
		>
			<p class="break-words">{loadError}</p>
			<Button variant="secondary" icon="refresh" class="mt-3" onclick={() => void load()}>
				Try again
			</Button>
		</div>
	{:else}
		<div class="space-y-6">
			<Switch
				bind:checked={readonly}
				label="Read-only"
				description="Only tools marked read-only can run."
			/>

			<section>
				<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Blocked tools</h3>
				{#if deny.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each deny as pattern (pattern)}
							<span
								class="inline-flex items-center gap-1 rounded-full bg-zinc-100 py-1 pr-1 pl-2.5 font-mono text-xs text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200"
							>
								{pattern}
								<button
									type="button"
									class="-my-2.5 inline-flex h-11 w-6 items-center justify-center rounded text-zinc-500 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:hover:text-zinc-100"
									aria-label={`Remove ${pattern}`}
									onclick={() => (deny = deny.filter((item) => item !== pattern))}
								>
									<Icon name="x" class="size-3" />
								</button>
							</span>
						{/each}
					</div>
				{:else}
					<p class="mt-2 text-sm text-zinc-500">No blocked tools.</p>
				{/if}

				<form
					class="mt-3 flex gap-2"
					onsubmit={(event) => {
						event.preventDefault();
						addDeny();
					}}
				>
					<input
						bind:value={denyInput}
						type="text"
						class={fieldClasses}
						placeholder="create-*"
						aria-label="Blocked tool pattern"
						autocomplete="off"
						spellcheck="false"
					/>
					<Button type="submit" variant="secondary" disabled={!denyInput.trim()}>Add</Button>
				</form>
				<p class="mt-1.5 text-xs text-zinc-500">
					Examples: <code class="font-mono">create-*</code>,
					<code class="font-mono">delete_*</code>
				</p>
			</section>

			<section>
				<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Only allow</h3>
				<p class="mt-0.5 text-xs text-zinc-500">Leave empty to allow every tool.</p>

				{#if allow.length > 0}
					<div class="mt-2 flex flex-wrap gap-2">
						{#each allow as pattern (pattern)}
							<span
								class="inline-flex items-center gap-1 rounded-full bg-zinc-100 py-1 pr-1 pl-2.5 font-mono text-xs text-zinc-700 dark:bg-zinc-800 dark:text-zinc-200"
							>
								{pattern}
								<button
									type="button"
									class="-my-2.5 inline-flex h-11 w-6 items-center justify-center rounded text-zinc-500 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:hover:text-zinc-100"
									aria-label={`Remove ${pattern}`}
									onclick={() => (allow = allow.filter((item) => item !== pattern))}
								>
									<Icon name="x" class="size-3" />
								</button>
							</span>
						{/each}
					</div>
				{:else}
					<p class="mt-2 text-sm text-zinc-500">Every tool is allowed.</p>
				{/if}

				<form
					class="mt-3 flex gap-2"
					onsubmit={(event) => {
						event.preventDefault();
						addAllow();
					}}
				>
					<input
						bind:value={allowInput}
						type="text"
						class={fieldClasses}
						placeholder="get_*"
						aria-label="Allowed tool pattern"
						autocomplete="off"
						spellcheck="false"
					/>
					<Button type="submit" variant="secondary" disabled={!allowInput.trim()}>Add</Button>
				</form>
				<p class="mt-1.5 text-xs text-zinc-500">
					Examples: <code class="font-mono">get_*</code>,
					<code class="font-mono">list-*</code>
				</p>
			</section>

			{#if apiError}
				<div
					class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm break-words text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
				>
					{apiError}
				</div>
			{/if}
		</div>
	{/if}

	{#snippet footer()}
		<Button variant="secondary" onclick={onclose} disabled={saving}>Cancel</Button>
		<Button loading={saving} disabled={loading || Boolean(loadError)} onclick={save}>
			Save rules
		</Button>
	{/snippet}
</Panel>
