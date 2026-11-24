<script lang="ts">
	import { untrack } from 'svelte';
	import { ApiError, errorMessage } from '#lib/shared/api/client.ts';
	import {
		Button,
		ConfirmDialog,
		Field,
		Icon,
		Panel,
		Pill,
		fieldClasses,
		selectClasses,
		showToast
	} from '#lib/shared/ui/index.ts';
	import { callTool, removeCommandTool } from '../api.ts';
	import { formatToolText, requestUrl } from '../http-tool.ts';
	import { initialArguments, listToolArguments } from '../tool-arguments.ts';
	import type { CallToolResult, JsonValue, ServiceView, ToolContent, ToolInfo } from '../types.ts';

	let {
		service,
		tool,
		onclose,
		onremoved,
		oneditrules
	}: {
		service: ServiceView;
		tool: ToolInfo;
		onclose: () => void;
		onremoved: () => void;
		oneditrules: (ref: string) => void;
	} = $props();

	const argumentsList = $derived(listToolArguments(tool.inputSchema));
	const readOnly = $derived(tool.annotations?.readOnlyHint === true);
	const httpRequest = $derived(
		service.command_tools?.find((entry) => entry.name === tool.name)?.http
	);
	const httpUrl = $derived(httpRequest ? requestUrl(service, httpRequest) : '');
	const httpDetails = $derived.by(() => {
		if (!httpRequest) return [];

		const groups = [
			{ label: 'query', keys: Object.keys(httpRequest.query ?? {}) },
			{ label: 'form', keys: Object.keys(httpRequest.form ?? {}) },
			{ label: 'json', keys: Object.keys(httpRequest.json ?? {}) },
			{ label: 'headers', keys: Object.keys(httpRequest.headers ?? {}) }
		];

		return groups
			.filter((group) => group.keys.length > 0)
			.map((group) => `${group.label}: ${group.keys.join(', ')}`);
	});
	const defaultAccount = untrack(
		() =>
			service.accounts.find((account) => account.default)?.name ?? service.accounts[0]?.name ?? ''
	);

	let accountName = $state(defaultAccount);
	let argumentsText = $state(
		untrack(() => JSON.stringify(initialArguments(tool.inputSchema), null, 2))
	);
	let jsonError = $state('');
	let running = $state(false);
	let removing = $state(false);
	let result = $state<CallToolResult | null>(null);
	let requestError = $state('');
	let blocked = $state(false);
	let formatModes = $state<Record<string, 'formatted' | 'raw'>>({});
	let confirmRun = $state(false);
	let confirmRemove = $state(false);

	const accountRef = $derived(`${service.name}/${accountName}`);
	const runLabel = $derived(readOnly ? 'Run' : 'Run (changes data)');
	const hasResult = $derived(Boolean(result || requestError));
	const failed = $derived(Boolean(requestError) || Boolean(result?.isError));

	function parseArguments(): Record<string, JsonValue> | null {
		jsonError = '';
		let parsed: unknown;

		try {
			parsed = JSON.parse(argumentsText);
		} catch {
			jsonError = 'Enter valid JSON.';
			return null;
		}

		if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed)) {
			jsonError = 'Arguments must be a JSON object.';
			return null;
		}

		return parsed as Record<string, JsonValue>;
	}

	async function runNow(): Promise<void> {
		const args = parseArguments();
		if (!args) return;

		running = true;
		result = null;
		requestError = '';
		blocked = false;
		formatModes = {};

		try {
			result = await callTool(accountRef, tool.name, args);
		} catch (cause) {
			requestError = errorMessage(cause);
			blocked = cause instanceof ApiError && cause.status === 403;
		} finally {
			running = false;
		}
	}

	function requestRun(): void {
		if (readOnly) {
			void runNow();
			return;
		}

		const args = parseArguments();
		if (!args) return;
		confirmRun = true;
	}

	function confirmRunNow(): void {
		confirmRun = false;
		void runNow();
	}

	async function remove(): Promise<void> {
		if (removing) return;

		removing = true;

		try {
			await removeCommandTool(service.name, tool.name);
			showToast(`Removed ${tool.name} from ${service.name}.`);
			confirmRemove = false;
			onremoved();
		} catch (cause) {
			showToast(errorMessage(cause), 'error');
			confirmRemove = false;
		} finally {
			removing = false;
		}
	}

	function contentKey(content: ToolContent, index: number): string {
		return `${index}-${content.type}`;
	}

	function setFormat(key: string, mode: 'formatted' | 'raw'): void {
		formatModes = { ...formatModes, [key]: mode };
	}

	async function copyText(text: string): Promise<void> {
		try {
			await navigator.clipboard.writeText(text);
			showToast('Copied');
		} catch {
			showToast('Could not copy', 'error');
		}
	}
</script>

<Panel title={tool.name} {onclose}>
	<div class="space-y-6">
		{#if tool.description}
			<p class="text-sm whitespace-pre-wrap text-zinc-600 dark:text-zinc-300">
				{tool.description}
			</p>
		{/if}

		{#if httpRequest}
			<section>
				<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Request</h3>
				<div
					class="mt-2 rounded-lg border border-zinc-200 bg-zinc-50 p-3 dark:border-zinc-800 dark:bg-zinc-950/60"
				>
					<div class="flex min-w-0 flex-wrap items-start gap-2">
						<Pill tone="brand">{httpRequest.method}</Pill>
						<span
							class="min-w-0 flex-1 font-mono text-xs break-all text-zinc-700 dark:text-zinc-300"
						>
							{httpRequest.path}
						</span>
					</div>
					<p class="mt-1 min-w-0 truncate font-mono text-xs text-zinc-500" title={httpUrl}>
						{httpUrl}
					</p>
					{#if httpDetails.length > 0}
						<p class="mt-1 text-xs text-zinc-500">{httpDetails.join(' · ')}</p>
					{/if}
				</div>
			</section>
		{/if}

		<section>
			<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Arguments</h3>

			{#if argumentsList.length > 0}
				<div class="mt-2 overflow-x-auto rounded-lg border border-zinc-200 dark:border-zinc-800">
					<table class="w-full text-left text-xs">
						<thead class="bg-zinc-50 text-zinc-500 dark:bg-zinc-950/60">
							<tr>
								<th class="px-3 py-2 font-medium">Name</th>
								<th class="px-3 py-2 font-medium">Type</th>
								<th class="px-3 py-2 font-medium">Required</th>
								<th class="px-3 py-2 font-medium">Description</th>
							</tr>
						</thead>
						<tbody class="divide-y divide-zinc-200 dark:divide-zinc-800">
							{#each argumentsList as argument (argument.name)}
								<tr>
									<td class="px-3 py-2 font-mono text-zinc-800 dark:text-zinc-200">
										{argument.name}
									</td>
									<td class="px-3 py-2 font-mono text-zinc-500">{argument.type}</td>
									<td class="px-3 py-2">
										{#if argument.required}
											<Pill tone="warning">required</Pill>
										{:else}
											<span class="text-zinc-400">optional</span>
										{/if}
									</td>
									<td class="px-3 py-2 text-zinc-500">{argument.description || '—'}</td>
								</tr>
							{/each}
						</tbody>
					</table>
				</div>
			{:else}
				<p class="mt-2 text-sm text-zinc-500">No arguments.</p>
			{/if}
		</section>

		<section class="space-y-4">
			<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Try it</h3>

			{#if service.accounts.length > 0}
				<Field label="Account" id={`${tool.name}-account`}>
					<div class="relative">
						<select
							id={`${tool.name}-account`}
							bind:value={accountName}
							class={selectClasses}
							onchange={() => {
								result = null;
								requestError = '';
								blocked = false;
								formatModes = {};
							}}
						>
							{#each service.accounts as account (account.name)}
								<option value={account.name}>{account.name}</option>
							{/each}
						</select>
						<Icon
							name="chevron"
							class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
						/>
					</div>
				</Field>
			{:else}
				<p class="text-sm text-zinc-500">This service has no accounts.</p>
			{/if}

			<Field
				label="Arguments (JSON)"
				id={`${tool.name}-arguments`}
				error={jsonError}
				hint="Edit the JSON object before running the tool."
			>
				<textarea
					id={`${tool.name}-arguments`}
					bind:value={argumentsText}
					class={`${fieldClasses} h-40 resize-y py-2 font-mono text-xs leading-5`}
					spellcheck="false"
					disabled={running}></textarea>
			</Field>

			{#if hasResult}
				<div
					class={[
						'rounded-lg border p-3',
						failed
							? 'border-red-200 bg-red-50 dark:border-red-500/30 dark:bg-red-500/10'
							: 'border-emerald-200 bg-emerald-50 dark:border-emerald-500/30 dark:bg-emerald-500/10'
					].join(' ')}
				>
					{#if requestError}
						<p class="text-sm break-words text-red-700 dark:text-red-300">{requestError}</p>
						{#if blocked}
							<button
								type="button"
								class="mt-1 inline-flex min-h-11 items-center text-sm font-medium text-red-700 underline underline-offset-2 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-red-300"
								onclick={() => oneditrules(accountRef)}
							>
								Edit rules
							</button>
						{/if}
					{:else if result}
						{#if result.content.length === 0}
							<p class="text-sm text-zinc-600 dark:text-zinc-300">No output.</p>
						{/if}
						<div class="space-y-2">
							{#each result.content as content, index (contentKey(content, index))}
								{#if content.type === 'text' && content.text !== undefined}
									{@const key = contentKey(content, index)}
									{@const format = formatToolText(content.text)}
									{@const mode = formatModes[key] ?? 'formatted'}
									{@const shown =
										format.json && mode === 'formatted' ? format.formatted : content.text}
									<div class="min-w-0">
										<div class="mb-1.5 flex flex-wrap items-center justify-end gap-2">
											{#if format.json}
												<div
													class="inline-flex overflow-hidden rounded-lg border border-zinc-300 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
													role="group"
													aria-label="Result format"
												>
													<button
														type="button"
														class={[
															'inline-flex min-h-11 items-center justify-center px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
															mode === 'formatted'
																? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
																: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
														].join(' ')}
														aria-pressed={mode === 'formatted'}
														onclick={() => setFormat(key, 'formatted')}
													>
														Formatted
													</button>
													<button
														type="button"
														class={[
															'inline-flex min-h-11 items-center justify-center px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset',
															mode === 'raw'
																? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
																: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
														].join(' ')}
														aria-pressed={mode === 'raw'}
														onclick={() => setFormat(key, 'raw')}
													>
														Raw
													</button>
												</div>
											{/if}
											<Button
												variant="secondary"
												icon="copy"
												class="min-h-11 shrink-0 px-3"
												onclick={() => void copyText(shown)}
											>
												Copy
											</Button>
										</div>
										<pre
											class="max-h-80 overflow-auto rounded-md bg-zinc-950 p-3 font-mono text-xs break-words whitespace-pre-wrap text-zinc-100">{shown}</pre>
									</div>
								{:else if content.type === 'image' && content.data && content.mimeType}
									<img
										src={`data:${content.mimeType};base64,${content.data}`}
										alt="Tool output"
										class="max-h-80 max-w-full rounded-md border border-zinc-200 object-contain dark:border-zinc-700"
									/>
								{:else}
									<p class="text-xs text-zinc-500">Unsupported content: {content.type}</p>
								{/if}
							{/each}
						</div>
					{/if}
				</div>
			{/if}
		</section>
	</div>

	{#snippet footer()}
		{#if service.kind === 'command' || (service.kind === 'http' && tool.name !== 'http_get')}
			<Button
				variant="secondary"
				class="mr-auto"
				disabled={running || removing}
				onclick={() => (confirmRemove = true)}
			>
				Remove tool
			</Button>
		{/if}
		<Button
			loading={running}
			disabled={service.accounts.length === 0 || removing}
			onclick={requestRun}
		>
			{runLabel}
		</Button>
	{/snippet}
</Panel>

{#if confirmRun}
	<ConfirmDialog
		title="Run tool"
		message={`This tool may change data in ${accountRef}. Run it?`}
		confirmLabel="Run"
		confirmVariant="primary"
		onclose={() => (confirmRun = false)}
		onconfirm={confirmRunNow}
	/>
{/if}

{#if confirmRemove}
	<ConfirmDialog
		title="Remove tool"
		message={`Remove ${tool.name} from ${service.name}? This command tool will be deleted.`}
		confirmLabel="Remove"
		loading={removing}
		onclose={() => (confirmRemove = false)}
		onconfirm={() => void remove()}
	/>
{/if}
