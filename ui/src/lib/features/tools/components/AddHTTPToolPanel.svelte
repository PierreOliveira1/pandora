<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { validateName } from '#lib/features/accounts/schemas.ts';
	import {
		Button,
		Field,
		Icon,
		Panel,
		Switch,
		fieldClasses,
		selectClasses,
		showToast
	} from '#lib/shared/ui/index.ts';
	import { addCommandTool } from '../api.ts';
	import {
		buildHttpToolInput,
		findMissingParameters,
		findSecretEnv,
		keyValuesToRecord,
		requestPreview,
		type HttpBodyMode,
		type HttpKeyValue
	} from '../http-tool.ts';
	import type { CommandParam, CommandParamType, HttpToolMethod, ServiceView } from '../types.ts';

	type ParameterRow = {
		id: number;
		name: string;
		type: CommandParamType;
		required: boolean;
		description: string;
	};

	type KeyValueRow = HttpKeyValue & {
		id: number;
	};

	const methodOptions: HttpToolMethod[] = ['GET', 'HEAD', 'POST', 'PUT', 'PATCH', 'DELETE'];
	const bodyOptions: Array<{ value: HttpBodyMode; label: string }> = [
		{ value: 'none', label: 'None' },
		{ value: 'json', label: 'JSON' },
		{ value: 'form', label: 'Form' }
	];
	const typeOptions: Array<{ value: CommandParamType; label: string }> = [
		{ value: 'string', label: 'string' },
		{ value: 'number', label: 'number' },
		{ value: 'integer', label: 'integer' },
		{ value: 'boolean', label: 'boolean' },
		{ value: 'array', label: 'array' }
	];
	const pathHint = 'Use {{parameter}} for an argument and {{env.NAME}} for an account value.';
	const bodyHint = 'GET and HEAD have no body.';
	const jsonHint =
		'Dotted keys nest objects, like order.total. A value that is only {{parameter}} keeps its type.';
	const readOnlyHint = 'GET and HEAD tools are always read-only.';

	let {
		service,
		onclose,
		oncreated
	}: {
		service: ServiceView;
		onclose: () => void;
		oncreated: () => void;
	} = $props();

	const formId = $props.id();
	const urlOptions = $derived(Object.entries(service.urls ?? {}));
	let name = $state('');
	let description = $state('');
	let method = $state<HttpToolMethod>('GET');
	let url = $state(untrack(() => Object.keys(service.urls ?? {})[0] ?? ''));
	let path = $state('');
	let parameters = $state<ParameterRow[]>([]);
	let queryRows = $state<KeyValueRow[]>([]);
	let bodyRows = $state<KeyValueRow[]>([]);
	let headerRows = $state<KeyValueRow[]>([]);
	let bodyMode = $state<HttpBodyMode>('none');
	let readonly = $state(true);
	let timeout = $state('60');
	let errors = $state<Record<string, string>>({});
	let apiError = $state('');
	let submitting = $state(false);
	let panelBody = $state<HTMLElement | null>(null);
	let rowSequence = 0;

	const bodyDisabled = $derived(method === 'GET' || method === 'HEAD');
	const secretNames = $derived([
		...new Set(service.accounts.flatMap((account) => account.secret_env ?? []))
	]);
	const parameterNames = $derived(
		parameters.map((parameter) => parameter.name.trim()).filter(Boolean)
	);
	const queryRecord = $derived(
		keyValuesToRecord(queryRows.map(({ key, value }) => ({ key, value })))
	);
	const preview = $derived(
		requestPreview(service, {
			url,
			method,
			path,
			query: queryRecord
		})
	);

	function addParameter(): void {
		rowSequence += 1;
		parameters.push({
			id: rowSequence,
			name: '',
			type: 'string',
			required: false,
			description: ''
		});
	}

	function removeParameter(id: number): void {
		parameters = parameters.filter((parameter) => parameter.id !== id);
	}

	function addRow(rows: KeyValueRow[]): void {
		rowSequence += 1;
		rows.push({ id: rowSequence, key: '', value: '' });
	}

	function removeQueryRow(id: number): void {
		queryRows = queryRows.filter((row) => row.id !== id);
	}

	function removeBodyRow(id: number): void {
		bodyRows = bodyRows.filter((row) => row.id !== id);
	}

	function removeHeaderRow(id: number): void {
		headerRows = headerRows.filter((row) => row.id !== id);
	}

	function changeMethod(event: Event): void {
		const next = (event.currentTarget as HTMLSelectElement).value as HttpToolMethod;
		const wasBodyless = bodyDisabled;
		method = next;

		if (next === 'GET' || next === 'HEAD') {
			bodyMode = 'none';
			readonly = true;
		} else if (wasBodyless) {
			readonly = false;
		}
	}

	function validateRows(rows: KeyValueRow[]): string | null {
		const seen = new Set<string>();

		for (const row of rows) {
			const key = row.key.trim();
			if (!key) return 'Enter a key for each row.';
			if (seen.has(key)) return 'Keys must be unique.';
			seen.add(key);
		}

		return null;
	}

	function addReferenceErrors(
		next: Record<string, string>,
		section: string,
		values: string[]
	): void {
		if (values.length === 0) return;

		const secret = findSecretEnv(values, secretNames);
		if (secret) {
			next[section] =
				`${secret} is a secret: use it only in a service header (URLs and headers menu).`;
			return;
		}

		const missing = findMissingParameters(values, parameterNames);
		if (missing.length > 0) {
			next[section] = `Declare ${missing.map((name) => `{{${name}}}`).join(', ')} in Parameters.`;
		}
	}

	function validate(): Record<string, string> {
		const next: Record<string, string> = {};
		const nameError = validateName(name);
		if (nameError) next.name = nameError;

		if (urlOptions.length === 0) {
			next.url = 'This service has no URLs.';
		} else if (!url) {
			next.url = 'Choose a URL.';
		}

		const trimmedPath = path.trim();
		if (!trimmedPath) {
			next.path = 'Enter a path.';
		} else if (!trimmedPath.startsWith('/')) {
			next.path = 'Path must start with /.';
		}

		const timeoutValue = Number(timeout);
		if (!timeout.trim() || !Number.isInteger(timeoutValue) || timeoutValue < 1) {
			next.timeout = 'Enter a timeout in seconds.';
		}

		const seenParameters = new Set<string>();
		for (const parameter of parameters) {
			const parameterName = parameter.name.trim();
			if (!parameterName) {
				if (parameter.description.trim() || parameter.required) {
					next.parameters = 'Enter a name for each parameter.';
				}
				continue;
			}
			if (seenParameters.has(parameterName)) {
				next.parameters = 'Parameter names must be unique.';
			}
			seenParameters.add(parameterName);
		}

		const queryError = validateRows(queryRows);
		if (queryError) next.query = queryError;

		const headerError = validateRows(headerRows);
		if (headerError) next.headers = headerError;

		const bodyHasValues = !bodyDisabled && bodyMode !== 'none';
		if (bodyHasValues) {
			const bodyError = validateRows(bodyRows);
			if (bodyError) next.body = bodyError;
		}

		if (!next.path) addReferenceErrors(next, 'path', [trimmedPath]);
		if (!next.query) {
			addReferenceErrors(
				next,
				'query',
				queryRows.map((row) => row.value)
			);
		}
		if (bodyHasValues && !next.body) {
			addReferenceErrors(
				next,
				'body',
				bodyRows.map((row) => row.value)
			);
		}
		if (!next.headers) {
			addReferenceErrors(
				next,
				'headers',
				headerRows.map((row) => row.value)
			);
		}

		return next;
	}

	function paramsPayload(): Record<string, CommandParam> {
		const params: Record<string, CommandParam> = {};

		for (const parameter of parameters) {
			const parameterName = parameter.name.trim();
			if (!parameterName) continue;

			const param: CommandParam = { type: parameter.type };
			if (parameter.description.trim()) param.description = parameter.description.trim();
			if (parameter.required) param.required = true;
			params[parameterName] = param;
		}

		return params;
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		if (submitting) return;

		errors = {};
		apiError = '';
		const validation = validate();
		if (Object.keys(validation).length > 0) {
			errors = validation;
			return;
		}

		submitting = true;

		try {
			await addCommandTool(
				service.name,
				buildHttpToolInput({
					name,
					description,
					url,
					method,
					path,
					query: queryRows.map(({ key, value }) => ({ key, value })),
					headers: headerRows.map(({ key, value }) => ({ key, value })),
					bodyMode,
					body: bodyRows.map(({ key, value }) => ({ key, value })),
					parameters: parameters.map(({ name: parameterName, type, required, description }) => ({
						name: parameterName,
						type,
						required,
						description
					})),
					readonly,
					timeoutSec: Number(timeout)
				})
			);
			showToast(`Added ${name.trim()} to ${service.name}.`);
			oncreated();
		} catch (cause) {
			apiError = errorMessage(cause);
			await tick();
			panelBody?.scrollTo({ top: panelBody.scrollHeight, behavior: 'smooth' });
		} finally {
			submitting = false;
		}
	}
</script>

<Panel title="Add HTTP tool" {onclose} bind:body={panelBody}>
	<form id={formId} onsubmit={submit} class="space-y-6" novalidate>
		<fieldset disabled={submitting} class="min-w-0 space-y-6 border-0 p-0">
			<Field label="Name" id={`${formId}-name`} error={errors.name}>
				<input
					id={`${formId}-name`}
					bind:value={name}
					type="text"
					class={fieldClasses}
					placeholder="list-orders"
					autocomplete="off"
					aria-invalid={errors.name ? 'true' : undefined}
				/>
			</Field>

			<Field label="Description" id={`${formId}-description`} hint="Optional.">
				<input
					id={`${formId}-description`}
					bind:value={description}
					type="text"
					class={fieldClasses}
					placeholder="List the most recent orders"
					autocomplete="off"
				/>
			</Field>

			<section class="space-y-3">
				<h3 class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Request</h3>

				<div class="grid gap-3 sm:grid-cols-[7rem_minmax(0,1fr)]">
					<Field label="Method" id={`${formId}-method`}>
						<div class="relative">
							<select
								id={`${formId}-method`}
								value={method}
								class={selectClasses}
								onchange={changeMethod}
							>
								{#each methodOptions as option (option)}
									<option value={option}>{option}</option>
								{/each}
							</select>
							<Icon
								name="chevron"
								class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
							/>
						</div>
					</Field>

					{#if urlOptions.length === 1}
						<div class="min-w-0">
							<p class="mb-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-300">URL</p>
							<p class="flex min-h-11 min-w-0 items-center gap-2 font-mono text-xs text-zinc-500">
								<span class="shrink-0">{urlOptions[0][0]}</span>
								<span aria-hidden="true">·</span>
								<span class="min-w-0 truncate" title={urlOptions[0][1]}>{urlOptions[0][1]}</span>
							</p>
						</div>
					{:else}
						<Field label="URL" id={`${formId}-url`} error={errors.url}>
							<div class="relative">
								<select id={`${formId}-url`} bind:value={url} class={selectClasses}>
									{#each urlOptions as [urlName, urlValue] (urlName)}
										<option value={urlName}>{urlName} · {urlValue}</option>
									{/each}
								</select>
								<Icon
									name="chevron"
									class="pointer-events-none absolute top-1/2 right-3 size-4 -translate-y-1/2 text-zinc-500"
								/>
							</div>
						</Field>
					{/if}
				</div>

				<Field label="Path" id={`${formId}-path`} error={errors.path} hint={pathHint}>
					<input
						id={`${formId}-path`}
						bind:value={path}
						type="text"
						class={`${fieldClasses} font-mono text-xs`}
						placeholder={'/items/{{id}}'}
						autocomplete="off"
						spellcheck="false"
						aria-invalid={errors.path ? 'true' : undefined}
					/>
				</Field>

				<p
					class="min-w-0 rounded-lg bg-zinc-50 px-3 py-2 font-mono text-xs break-all text-zinc-500 dark:bg-zinc-950/60"
				>
					{preview}
				</p>
			</section>

			<section>
				<div class="flex items-center justify-between gap-3">
					<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Parameters</p>
					<Button variant="secondary" icon="plus" class="px-3" onclick={addParameter}>
						Add parameter
					</Button>
				</div>

				{#if parameters.length === 0}
					<p class="mt-2 text-sm text-zinc-500">No parameters yet.</p>
				{/if}

				<div class="mt-2 space-y-2">
					{#each parameters as parameter (parameter.id)}
						<div class="rounded-lg border border-zinc-200 p-2 dark:border-zinc-700">
							<div class="flex items-center gap-2">
								<div class="min-w-0 flex-1">
									<input
										bind:value={parameter.name}
										type="text"
										class={fieldClasses}
										placeholder="order_id"
										aria-label="Parameter name"
										autocomplete="off"
										spellcheck="false"
									/>
								</div>

								<div class="w-24 shrink-0 sm:w-32">
									<select
										bind:value={parameter.type}
										class={selectClasses}
										aria-label="Parameter type"
									>
										{#each typeOptions as option (option.value)}
											<option value={option.value}>{option.label}</option>
										{/each}
									</select>
								</div>

								<button
									type="button"
									class="inline-flex size-11 shrink-0 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
									aria-label="Remove parameter"
									onclick={() => removeParameter(parameter.id)}
								>
									<Icon name="x" class="size-5" />
								</button>
							</div>

							<div class="mt-2 flex items-center gap-3">
								<label
									class="flex min-h-11 shrink-0 items-center gap-2 text-sm text-zinc-600 dark:text-zinc-300"
								>
									<input
										bind:checked={parameter.required}
										type="checkbox"
										class="size-4 rounded border-zinc-300 accent-zinc-900 focus-visible:ring-2 focus-visible:ring-violet-500 dark:accent-zinc-100"
									/>
									Required
								</label>

								<div class="min-w-0 flex-1">
									<input
										bind:value={parameter.description}
										type="text"
										class={fieldClasses}
										placeholder="Description (optional)"
										aria-label="Parameter description"
										autocomplete="off"
									/>
								</div>
							</div>
						</div>
					{/each}
				</div>

				{#if errors.parameters}
					<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.parameters}</p>
				{/if}
			</section>

			<section>
				<div class="flex items-center justify-between gap-3">
					<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Query</p>
					<Button variant="secondary" icon="plus" class="px-3" onclick={() => addRow(queryRows)}>
						Add query parameter
					</Button>
				</div>

				{#if queryRows.length === 0}
					<p class="mt-2 text-sm text-zinc-500">No query parameters yet.</p>
				{/if}

				<div class="mt-2 space-y-2">
					{#each queryRows as row (row.id)}
						<div
							class="grid grid-cols-[minmax(0,1fr)_44px] gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_44px]"
						>
							<input
								bind:value={row.key}
								type="text"
								class={`${fieldClasses} col-span-2 font-mono text-xs sm:col-span-1`}
								placeholder="limit"
								aria-label="Query key"
								autocomplete="off"
								spellcheck="false"
							/>
							<input
								bind:value={row.value}
								type="text"
								class={`${fieldClasses} font-mono text-xs`}
								placeholder={'{{limit}}'}
								aria-label="Query value"
								autocomplete="off"
								spellcheck="false"
							/>
							<button
								type="button"
								class="inline-flex size-11 items-center justify-center justify-self-end rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none sm:justify-self-auto dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
								aria-label="Remove query parameter"
								onclick={() => removeQueryRow(row.id)}
							>
								<Icon name="x" class="size-5" />
							</button>
						</div>
					{/each}
				</div>

				{#if errors.query}
					<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.query}</p>
				{/if}
			</section>

			<section>
				<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Body</p>

				<div
					class="mt-2 inline-flex overflow-hidden rounded-lg border border-zinc-300 bg-white shadow-sm dark:border-zinc-700 dark:bg-zinc-900"
					role="group"
					aria-label="Body type"
				>
					{#each bodyOptions as option (option.value)}
						<button
							type="button"
							class={[
								'inline-flex min-h-11 items-center justify-center px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none focus-visible:ring-inset disabled:cursor-not-allowed disabled:opacity-50',
								bodyMode === option.value
									? 'bg-violet-50 text-violet-700 dark:bg-violet-500/15 dark:text-violet-300'
									: 'text-zinc-600 hover:bg-zinc-50 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
							].join(' ')}
							aria-pressed={bodyMode === option.value}
							disabled={bodyDisabled}
							onclick={() => (bodyMode = option.value)}
						>
							{option.label}
						</button>
					{/each}
				</div>

				{#if bodyDisabled}
					<p class="mt-2 text-xs text-zinc-500">{bodyHint}</p>
				{:else if bodyMode !== 'none'}
					{#if bodyMode === 'json'}
						<p class="mt-2 text-xs text-zinc-500">{jsonHint}</p>
					{/if}

					{#if bodyRows.length === 0}
						<p class="mt-2 text-sm text-zinc-500">No body fields yet.</p>
					{/if}

					<div class="mt-2 space-y-2">
						{#each bodyRows as row (row.id)}
							<div
								class="grid grid-cols-[minmax(0,1fr)_44px] gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_44px]"
							>
								<input
									bind:value={row.key}
									type="text"
									class={`${fieldClasses} col-span-2 font-mono text-xs sm:col-span-1`}
									placeholder="reason"
									aria-label="Body key"
									autocomplete="off"
									spellcheck="false"
								/>
								<input
									bind:value={row.value}
									type="text"
									class={`${fieldClasses} font-mono text-xs`}
									placeholder={'{{reason}}'}
									aria-label="Body value"
									autocomplete="off"
									spellcheck="false"
								/>
								<button
									type="button"
									class="inline-flex size-11 items-center justify-center justify-self-end rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none sm:justify-self-auto dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
									aria-label="Remove body field"
									onclick={() => removeBodyRow(row.id)}
								>
									<Icon name="x" class="size-5" />
								</button>
							</div>
						{/each}
					</div>

					<Button
						variant="secondary"
						icon="plus"
						class="mt-2 px-3"
						onclick={() => addRow(bodyRows)}
					>
						Add body field
					</Button>

					{#if errors.body}
						<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.body}</p>
					{/if}
				{/if}
			</section>

			<section>
				<div class="flex items-center justify-between gap-3">
					<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
						Request headers (optional)
					</p>
					<Button variant="secondary" icon="plus" class="px-3" onclick={() => addRow(headerRows)}>
						Add header
					</Button>
				</div>

				{#if headerRows.length === 0}
					<p class="mt-2 text-sm text-zinc-500">No request headers yet.</p>
				{/if}

				<div class="mt-2 space-y-2">
					{#each headerRows as row (row.id)}
						<div
							class="grid grid-cols-[minmax(0,1fr)_44px] gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_44px]"
						>
							<input
								bind:value={row.key}
								type="text"
								class={`${fieldClasses} col-span-2 font-mono text-xs sm:col-span-1`}
								placeholder="X-Tenant"
								aria-label="Header name"
								autocomplete="off"
								spellcheck="false"
							/>
							<input
								bind:value={row.value}
								type="text"
								class={`${fieldClasses} font-mono text-xs`}
								placeholder={'{{tenant}}'}
								aria-label="Header value"
								autocomplete="off"
								spellcheck="false"
							/>
							<button
								type="button"
								class="inline-flex size-11 items-center justify-center justify-self-end rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none sm:justify-self-auto dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
								aria-label="Remove header"
								onclick={() => removeHeaderRow(row.id)}
							>
								<Icon name="x" class="size-5" />
							</button>
						</div>
					{/each}
				</div>

				{#if errors.headers}
					<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.headers}</p>
				{/if}
			</section>

			<div class="space-y-4">
				<Switch
					bind:checked={readonly}
					label="Read-only"
					description={bodyDisabled ? readOnlyHint : undefined}
					disabled={bodyDisabled}
				/>
				<Field label="Timeout (seconds)" id={`${formId}-timeout`} error={errors.timeout}>
					<input
						id={`${formId}-timeout`}
						bind:value={timeout}
						type="number"
						min="1"
						step="1"
						class={fieldClasses}
						aria-invalid={errors.timeout ? 'true' : undefined}
					/>
				</Field>
			</div>
		</fieldset>

		{#if apiError}
			<div
				class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm break-words text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
			>
				{apiError}
			</div>
		{/if}
	</form>

	{#snippet footer()}
		<Button variant="secondary" onclick={onclose} disabled={submitting}>Cancel</Button>
		<Button type="submit" form={formId} loading={submitting}>Add tool</Button>
	{/snippet}
</Panel>
