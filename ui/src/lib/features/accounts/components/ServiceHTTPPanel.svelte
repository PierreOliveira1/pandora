<script lang="ts">
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, ConfirmDialog, Panel, fieldClasses, showToast } from '#lib/shared/ui/index.ts';
	import {
		removeServiceHeader,
		removeServiceUrl,
		setServiceHeader,
		setServiceUrl
	} from '../api.ts';
	import type { ServiceView } from '../types.ts';

	type RemoveTarget = { type: 'url' | 'header'; name: string };

	let {
		service,
		onclose,
		onchanged
	}: {
		service: ServiceView;
		onclose: () => void;
		onchanged: () => Promise<void>;
	} = $props();

	let urlName = $state('');
	let urlValue = $state('');
	let headerName = $state('');
	let headerValue = $state('');
	let savingUrl = $state(false);
	let savingHeader = $state(false);
	let urlError = $state('');
	let headerError = $state('');
	let removeTarget = $state<RemoveTarget | null>(null);
	let removing = $state(false);

	const urls = $derived(
		Object.entries(service.urls ?? {}).sort(([left], [right]) => left.localeCompare(right))
	);
	const headers = $derived(
		Object.entries(service.headers ?? {}).sort(([left], [right]) => left.localeCompare(right))
	);
	const removeMessage = $derived.by(() => {
		if (!removeTarget) return '';
		if (removeTarget.type === 'url') {
			return `Remove URL ${removeTarget.name} from ${service.name}?`;
		}
		return `Remove header ${removeTarget.name} from ${service.name}?`;
	});

	async function saveURL(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (savingUrl) return;

		urlError = '';
		const name = urlName.trim();
		const url = urlValue.trim();

		if (!name || !url) {
			urlError = 'Enter a name and a URL.';
			return;
		}

		savingUrl = true;

		try {
			await setServiceUrl(service.name, name, url);
			showToast(`URL saved for ${service.name}.`);
			urlName = '';
			urlValue = '';
			await onchanged();
		} catch (cause) {
			urlError = errorMessage(cause);
		} finally {
			savingUrl = false;
		}
	}

	async function saveHeader(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (savingHeader) return;

		headerError = '';
		const name = headerName.trim();
		const template = headerValue.trim();

		if (!name || !template) {
			headerError = 'Enter a name and a template.';
			return;
		}

		savingHeader = true;

		try {
			await setServiceHeader(service.name, name, template);
			showToast(`Header saved for ${service.name}.`);
			headerName = '';
			headerValue = '';
			await onchanged();
		} catch (cause) {
			headerError = errorMessage(cause);
		} finally {
			savingHeader = false;
		}
	}

	async function confirmRemove(): Promise<void> {
		const target = removeTarget;
		if (!target || removing) return;

		removing = true;

		try {
			if (target.type === 'url') {
				await removeServiceUrl(service.name, target.name);
				showToast(`URL ${target.name} removed.`);
			} else {
				await removeServiceHeader(service.name, target.name);
				showToast(`Header ${target.name} removed.`);
			}

			removeTarget = null;
			await onchanged();
		} catch (cause) {
			if (target.type === 'url') {
				urlError = errorMessage(cause);
			} else {
				headerError = errorMessage(cause);
			}
			removeTarget = null;
		} finally {
			removing = false;
		}
	}

	function closePanel(): void {
		if (!removeTarget) onclose();
	}
</script>

<Panel title={`URLs and headers · ${service.name}`} onclose={closePanel}>
	<div class="space-y-8">
		<section>
			<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Base URLs</h3>

			{#if urls.length > 0}
				<ul class="mt-2 divide-y divide-zinc-200 dark:divide-zinc-800">
					{#each urls as [name, url] (name)}
						<li class="flex min-h-11 items-center gap-3 py-2">
							<span class="shrink-0 font-mono text-xs font-medium">{name}</span>
							<span class="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500" title={url}>
								{url}
							</span>
							<Button
								variant="ghost"
								class="min-h-11 shrink-0 px-3"
								onclick={() => (removeTarget = { type: 'url', name })}
							>
								Remove
							</Button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="mt-2 text-sm text-zinc-500">No URLs yet.</p>
			{/if}

			<form class="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr_auto]" onsubmit={saveURL} novalidate>
				<input
					bind:value={urlName}
					type="text"
					class={fieldClasses}
					placeholder="api"
					aria-label="URL name"
					autocomplete="off"
					spellcheck="false"
				/>
				<input
					bind:value={urlValue}
					type="url"
					class={fieldClasses}
					placeholder="https://api.example.com/v1"
					aria-label="Base URL"
					autocomplete="off"
					spellcheck="false"
				/>
				<Button type="submit" variant="secondary" class="w-full sm:w-auto" loading={savingUrl}>
					Save
				</Button>
			</form>
			{#if urlError}
				<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{urlError}</p>
			{/if}
			<p class="mt-1.5 text-xs text-zinc-500">Saving an existing name replaces its URL.</p>
		</section>

		<section>
			<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Headers</h3>

			{#if headers.length > 0}
				<ul class="mt-2 divide-y divide-zinc-200 dark:divide-zinc-800">
					{#each headers as [name, template] (name)}
						<li class="flex min-h-11 items-center gap-3 py-2">
							<span class="shrink-0 font-mono text-xs font-medium">{name}</span>
							<span
								class="min-w-0 flex-1 truncate font-mono text-xs text-zinc-500"
								title={template}
							>
								{template}
							</span>
							<Button
								variant="ghost"
								class="min-h-11 shrink-0 px-3"
								onclick={() => (removeTarget = { type: 'header', name })}
							>
								Remove
							</Button>
						</li>
					{/each}
				</ul>
			{:else}
				<p class="mt-2 text-sm text-zinc-500">No headers yet.</p>
			{/if}

			<form class="mt-3 grid gap-2 sm:grid-cols-[8rem_1fr_auto]" onsubmit={saveHeader} novalidate>
				<input
					bind:value={headerName}
					type="text"
					class={fieldClasses}
					placeholder="Authorization"
					aria-label="Header name"
					autocomplete="off"
					spellcheck="false"
				/>
				<input
					bind:value={headerValue}
					type="text"
					class={fieldClasses}
					placeholder={'Bearer {{env.API_TOKEN}}'}
					aria-label="Header template"
					autocomplete="off"
					spellcheck="false"
				/>
				<Button type="submit" variant="secondary" class="w-full sm:w-auto" loading={savingHeader}>
					Save
				</Button>
			</form>
			{#if headerError}
				<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{headerError}</p>
			{/if}
			<p class="mt-1.5 text-xs text-zinc-500">
				Use <code class="font-mono">{'{{env.NAME}}'}</code> to insert an account value; the config keeps
				only the template.
			</p>
		</section>
	</div>
</Panel>

{#if removeTarget}
	<ConfirmDialog
		title={removeTarget.type === 'url' ? 'Remove URL' : 'Remove header'}
		message={removeMessage}
		loading={removing}
		onclose={() => {
			if (!removing) removeTarget = null;
		}}
		onconfirm={() => void confirmRemove()}
	/>
{/if}
