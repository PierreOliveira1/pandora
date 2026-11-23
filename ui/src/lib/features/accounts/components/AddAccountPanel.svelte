<script lang="ts">
	import { tick, untrack } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Field, Icon, Panel, fieldClasses, selectClasses } from '#lib/shared/ui/index.ts';
	import { addAccount, loadStatus, startLogin, waitForLogin } from '../api.ts';
	import { parseCommandLine } from '../parse-command.ts';
	import { validateName, validateSpec, validateUrl } from '../schemas.ts';
	import { suggestServiceName } from '../suggest-name.ts';
	import type {
		AddAccountInput,
		AddAccountResponse,
		ServiceKind,
		ServiceView,
		StatusInfo
	} from '../types.ts';
	import AddAccountResult from './AddAccountResult.svelte';

	type Mode = 'new' | 'existing';
	type SignInMode = 'auto' | 'oauth' | 'header';
	type HeaderRow = { id: number; name: string; value: string };
	type EnvRow = { id: number; key: string; value: string; secret: boolean };

	const kindOptions: Array<{ kind: ServiceKind; title: string; description: string }> = [
		{
			kind: 'mcp-http',
			title: 'Remote MCP',
			description: 'An MCP server URL, like https://mcp.example.com/mcp'
		},
		{
			kind: 'mcp-stdio',
			title: 'Local MCP',
			description: 'A command that starts an MCP server, like npx -y @scope/server'
		},
		{
			kind: 'openapi',
			title: 'OpenAPI',
			description: 'Any HTTP API described by an OpenAPI or Swagger spec'
		},
		{
			kind: 'command',
			title: 'Commands',
			description: 'Your own terminal commands, added as tools later'
		}
	];

	const signInOptions: Array<{ value: SignInMode; label: string }> = [
		{ value: 'auto', label: 'Detect automatically' },
		{ value: 'oauth', label: 'OAuth with my own app' },
		{ value: 'header', label: 'Token or API key header' }
	];

	let { services, onclose }: { services: ServiceView[]; onclose: () => void } = $props();

	const formId = $props.id();
	let mode = $state<Mode>('new');
	let kind = $state<ServiceKind | null>(null);
	let existingService = $state(untrack(() => services[0]?.name ?? ''));
	let serviceName = $state('');
	let serviceNameTouched = $state(false);
	let serviceDescription = $state('');
	let accountName = $state('default');
	let accountDescription = $state('');
	let url = $state('');
	let spec = $state('');
	let apiBaseUrl = $state('');
	let command = $state('');
	let signIn = $state<SignInMode>('auto');
	let clientId = $state('');
	let clientSecret = $state('');
	let headerRows = $state<HeaderRow[]>([{ id: 1, name: '', value: '' }]);
	let envRows = $state<EnvRow[]>([]);
	let errors = $state<Record<string, string>>({});
	let apiError = $state('');
	let submitting = $state(false);
	let result = $state<AddAccountResponse | null>(null);
	let createdService = $state('');
	let createdAccount = $state('');
	let loginWaiting = $state(false);
	let loginError = $state('');
	let refreshedStatus = $state<StatusInfo | null>(null);
	let panelBody = $state<HTMLElement | null>(null);
	let rowSequence = 1;

	const selectedService = $derived(services.find((service) => service.name === existingService));
	const parsedCommand = $derived(parseCommandLine(command));
	const showHeaders = $derived(
		mode === 'new'
			? (kind === 'mcp-http' && signIn === 'header') || kind === 'openapi'
			: Boolean(
					selectedService &&
					(selectedService.kind === 'mcp-http' || selectedService.kind === 'openapi') &&
					selectedService.auth === 'header'
				)
	);
	const showEnv = $derived(
		mode === 'new' ? kind === 'mcp-stdio' : selectedService?.kind === 'mcp-stdio'
	);
	const showOAuthFields = $derived(mode === 'new' && kind === 'mcp-http' && signIn === 'oauth');
	const needsHeaderCredentials = $derived(
		showHeaders &&
			(mode === 'existing' || (mode === 'new' && kind === 'mcp-http' && signIn === 'header'))
	);
	const resultStatus = $derived(refreshedStatus ?? result?.status ?? null);
	const showResultLogin = $derived(
		Boolean(
			result && resultStatus && !resultStatus.ok && resultStatus.needs_login && !refreshedStatus
		)
	);

	$effect(() => {
		if (mode !== 'new' || serviceNameTouched) return;

		const suggestion = suggestServiceName({ kind, url, spec, apiBaseUrl, command });
		if (suggestion !== serviceName) serviceName = suggestion;
	});

	function selectMode(next: Mode): void {
		mode = next;
		errors = {};
		apiError = '';

		if (next === 'existing' && services.length > 0 && !selectedService) {
			existingService = services[0].name;
		}
	}

	function selectKind(next: ServiceKind): void {
		kind = next;
		errors = {};
		apiError = '';
	}

	function addHeader(): void {
		rowSequence += 1;
		headerRows.push({ id: rowSequence, name: '', value: '' });
	}

	function removeHeader(id: number): void {
		headerRows = headerRows.filter((row) => row.id !== id);
	}

	function addEnvironmentVariable(): void {
		rowSequence += 1;
		envRows.push({ id: rowSequence, key: '', value: '', secret: false });
	}

	function removeEnvironmentVariable(id: number): void {
		envRows = envRows.filter((row) => row.id !== id);
	}

	function toggleSecret(row: EnvRow): void {
		row.secret = !row.secret;
	}

	function headersPayload(): Record<string, string> {
		const headers: Record<string, string> = {};

		for (const row of headerRows) {
			const name = row.name.trim();
			if (!name) continue;
			headers[name] = row.value.trim();
		}

		return headers;
	}

	function envPayload(): Pick<AddAccountInput, 'env' | 'secret_env'> {
		const env: Record<string, string> = {};
		const secret_env: Record<string, string> = {};

		for (const row of envRows) {
			const key = row.key.trim();
			if (!key) continue;
			if (row.secret) {
				secret_env[key] = row.value;
			} else {
				env[key] = row.value;
			}
		}

		const payload: Pick<AddAccountInput, 'env' | 'secret_env'> = {};
		if (Object.keys(env).length > 0) payload.env = env;
		if (Object.keys(secret_env).length > 0) payload.secret_env = secret_env;
		return payload;
	}

	function validate(): Record<string, string> {
		const next: Record<string, string> = {};

		if (mode === 'new') {
			const serviceError = validateName(serviceName);
			if (serviceError) next.serviceName = serviceError;
			if (!kind) next.kind = 'Choose a source type.';
		} else if (!selectedService) {
			next.existing = 'Choose a service.';
		}

		const accountError = validateName(accountName);
		if (accountError) next.accountName = accountError;

		if (mode === 'new' && kind === 'mcp-http') {
			const urlError = validateUrl(url);
			if (urlError) next.url = urlError;
			if (signIn === 'oauth' && !clientId.trim()) {
				next.clientId = 'Enter your OAuth client ID.';
			}
		}

		if (mode === 'new' && kind === 'openapi') {
			const specError = validateSpec(spec);
			if (specError) next.spec = specError;
			if (apiBaseUrl.trim()) {
				const baseError = validateUrl(apiBaseUrl);
				if (baseError) next.baseUrl = baseError;
			}
		}

		if (mode === 'new' && kind === 'mcp-stdio') {
			if (!command.trim()) {
				next.command = 'Enter the command that starts the server.';
			} else if (parsedCommand.length === 0 || !parsedCommand[0]) {
				next.command = 'Enter at least one command argument.';
			}
		}

		if (showHeaders) {
			const rows = headerRows.map((row) => ({ name: row.name.trim(), value: row.value.trim() }));
			const incomplete = rows.some((row) => (row.name && !row.value) || (!row.name && row.value));
			const complete = rows.filter((row) => row.name && row.value);

			if (incomplete) {
				next.headers = 'Fill both the header name and value, or remove the row.';
			} else if (needsHeaderCredentials && complete.length === 0) {
				next.headers = 'Add at least one header.';
			}
		}

		if (showEnv) {
			const rows = envRows.map((row) => ({ key: row.key.trim(), value: row.value.trim() }));
			const incomplete = rows.some((row) => (row.key && !row.value) || (!row.key && row.value));

			if (incomplete) {
				next.env = 'Fill both the variable name and value, or remove the row.';
			}
		}

		return next;
	}

	function buildInput(service: string, account: string): AddAccountInput {
		const payload: AddAccountInput = {
			service,
			account,
			description: accountDescription.trim() || undefined
		};

		if (mode === 'existing') {
			if (showHeaders) payload.secret_headers = headersPayload();
			if (showEnv) Object.assign(payload, envPayload());
			return payload;
		}

		if (!kind) return payload;

		payload.kind = kind;
		payload.service_description = serviceDescription.trim() || undefined;

		if (kind === 'mcp-http') {
			payload.url = url.trim();
			if (signIn === 'oauth') {
				payload.auth = 'oauth';
				payload.oauth_client_id = clientId.trim();
				if (clientSecret.trim()) payload.oauth_client_secret = clientSecret;
			} else if (signIn === 'header') {
				const headers = headersPayload();
				if (Object.keys(headers).length > 0) payload.secret_headers = headers;
			}
		}

		if (kind === 'openapi') {
			payload.spec = spec.trim();
			if (apiBaseUrl.trim()) payload.url = apiBaseUrl.trim();
			const headers = headersPayload();
			if (Object.keys(headers).length > 0) payload.secret_headers = headers;
		}

		if (kind === 'mcp-stdio') {
			payload.command = parsedCommand;
			Object.assign(payload, envPayload());
		}

		return payload;
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

		const service = mode === 'new' ? serviceName.trim() : existingService;
		const account = accountName.trim();
		submitting = true;

		try {
			const response = await addAccount(buildInput(service, account));
			createdService = service;
			createdAccount = account;
			result = response;
			refreshedStatus = null;
			loginError = '';
		} catch (cause) {
			apiError = errorMessage(cause);
			await tick();
			panelBody?.scrollTo({ top: panelBody.scrollHeight, behavior: 'smooth' });
		} finally {
			submitting = false;
		}
	}

	async function loginResult(): Promise<void> {
		if (!result) return;

		loginWaiting = true;
		loginError = '';

		try {
			const login = await startLogin(createdService, createdAccount);
			await waitForLogin(login.login_id);

			try {
				const statuses = await loadStatus(result.ref);
				refreshedStatus = statuses.find((status) => status.ref === result?.ref) ?? {
					ref: result.ref,
					ok: true,
					needs_login: false,
					tools: result.status.tools,
					latency_ms: 0
				};
			} catch {
				refreshedStatus = {
					ref: result.ref,
					ok: true,
					needs_login: false,
					tools: result.status.tools,
					latency_ms: 0
				};
			}
		} catch (cause) {
			loginError = errorMessage(cause);
		} finally {
			loginWaiting = false;
		}
	}
</script>

{#snippet accountFields()}
	<div class="space-y-4">
		<Field label="Account name" id={`${formId}-account-name`} error={errors.accountName}>
			<input
				id={`${formId}-account-name`}
				bind:value={accountName}
				type="text"
				class={fieldClasses}
				placeholder="default"
				autocomplete="off"
				aria-invalid={errors.accountName ? 'true' : undefined}
			/>
		</Field>

		<Field label="Account description" id={`${formId}-account-description`} hint="Optional.">
			<input
				id={`${formId}-account-description`}
				bind:value={accountDescription}
				type="text"
				class={fieldClasses}
				placeholder="e.g. Personal tasks"
				autocomplete="off"
			/>
		</Field>
	</div>
{/snippet}

<Panel title="Add account" {onclose} bind:body={panelBody}>
	{#if result && resultStatus}
		<AddAccountResult
			accountRef={result.ref}
			status={resultStatus}
			warning={result.warning}
			{loginWaiting}
			{loginError}
		/>
	{:else}
		<form id={formId} onsubmit={submit} class="space-y-6" novalidate>
			<fieldset disabled={submitting} class="min-w-0 space-y-6 border-0 p-0">
				<div>
					<p class="mb-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-300">Service</p>
					<div
						class="flex gap-1 rounded-lg bg-zinc-100 p-1 dark:bg-zinc-800"
						role="group"
						aria-label="Service"
					>
						<button
							type="button"
							class={[
								'inline-flex min-h-11 flex-1 items-center justify-center rounded-md px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none',
								mode === 'new'
									? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
									: 'text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'
							].join(' ')}
							aria-pressed={mode === 'new'}
							onclick={() => selectMode('new')}
						>
							New service
						</button>
						{#if services.length > 0}
							<button
								type="button"
								class={[
									'inline-flex min-h-11 flex-1 items-center justify-center rounded-md px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none',
									mode === 'existing'
										? 'bg-white text-zinc-900 shadow-sm dark:bg-zinc-950 dark:text-zinc-100'
										: 'text-zinc-500 hover:text-zinc-800 dark:hover:text-zinc-200'
								].join(' ')}
								aria-pressed={mode === 'existing'}
								onclick={() => selectMode('existing')}
							>
								Existing service
							</button>
						{/if}
					</div>
				</div>

				{#if mode === 'new'}
					<div>
						<p class="mb-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-300">Source type</p>
						<div class="grid gap-3 sm:grid-cols-2">
							{#each kindOptions as option (option.kind)}
								<button
									type="button"
									class={[
										'rounded-xl border p-3 text-left transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none',
										kind === option.kind
											? 'border-violet-600 bg-violet-50 dark:border-violet-500 dark:bg-violet-500/10'
											: 'border-zinc-200 bg-white hover:border-zinc-300 hover:bg-zinc-50 dark:border-zinc-700 dark:bg-zinc-900 dark:hover:border-zinc-600 dark:hover:bg-zinc-800'
									].join(' ')}
									aria-pressed={kind === option.kind}
									onclick={() => selectKind(option.kind)}
								>
									<span class="flex items-center justify-between gap-2">
										<span class="text-sm font-medium">{option.title}</span>
										{#if kind === option.kind}
											<Icon
												name="check"
												class="size-4 shrink-0 text-violet-600 dark:text-violet-400"
											/>
										{/if}
									</span>
									<span class="mt-1 block text-xs text-zinc-500">{option.description}</span>
								</button>
							{/each}
						</div>
						{#if errors.kind}
							<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.kind}</p>
						{/if}
					</div>
				{:else}
					<Field label="Choose a service" id={`${formId}-existing-service`} error={errors.existing}>
						<div class="relative">
							<select
								id={`${formId}-existing-service`}
								bind:value={existingService}
								class={selectClasses}
								onchange={() => (errors = {})}
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
					</Field>
					{@render accountFields()}
				{/if}

				{#if mode === 'new' && kind === 'mcp-http'}
					<div class="space-y-4">
						<Field label="URL" id={`${formId}-url`} error={errors.url}>
							<input
								id={`${formId}-url`}
								bind:value={url}
								type="url"
								class={fieldClasses}
								placeholder="https://mcp.example.com/mcp"
								autocomplete="off"
								aria-invalid={errors.url ? 'true' : undefined}
							/>
						</Field>

						<fieldset class="min-w-0 border-0 p-0">
							<legend class="mb-1.5 text-sm font-medium text-zinc-700 dark:text-zinc-300">
								Sign-in
							</legend>
							<div class="space-y-2">
								{#each signInOptions as option (option.value)}
									<label
										class="flex min-h-11 cursor-pointer items-center gap-3 rounded-lg border border-zinc-200 bg-white px-3 py-2 text-sm transition-colors hover:bg-zinc-50 has-[:checked]:border-zinc-900 dark:border-zinc-700 dark:bg-zinc-900 dark:hover:bg-zinc-800 dark:has-[:checked]:border-zinc-100"
									>
										<input
											type="radio"
											name="sign-in"
											value={option.value}
											checked={signIn === option.value}
											onchange={() => (signIn = option.value)}
											class="size-4 accent-zinc-900 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:ring-offset-2 focus-visible:outline-none dark:accent-zinc-100"
										/>
										<span>{option.label}</span>
									</label>
								{/each}
							</div>
						</fieldset>

						{#if showOAuthFields}
							<div class="space-y-4 rounded-lg bg-zinc-50 p-3 dark:bg-zinc-950/60">
								<Field label="Client ID" id={`${formId}-client-id`} error={errors.clientId}>
									<input
										id={`${formId}-client-id`}
										bind:value={clientId}
										type="text"
										class={fieldClasses}
										autocomplete="off"
										aria-invalid={errors.clientId ? 'true' : undefined}
									/>
								</Field>

								<Field label="Client secret" id={`${formId}-client-secret`} hint="Optional.">
									<input
										id={`${formId}-client-secret`}
										bind:value={clientSecret}
										type="password"
										class={fieldClasses}
										autocomplete="off"
									/>
								</Field>
							</div>
						{/if}
					</div>
				{/if}

				{#if mode === 'new' && kind === 'openapi'}
					<div class="space-y-4">
						<Field label="Spec URL or file path" id={`${formId}-spec`} error={errors.spec}>
							<input
								id={`${formId}-spec`}
								bind:value={spec}
								type="text"
								class={fieldClasses}
								placeholder="https://api.example.com/openapi.json"
								autocomplete="off"
								aria-invalid={errors.spec ? 'true' : undefined}
							/>
						</Field>

						<Field
							label="API base URL"
							id={`${formId}-base-url`}
							error={errors.baseUrl}
							hint="Optional."
						>
							<input
								id={`${formId}-base-url`}
								bind:value={apiBaseUrl}
								type="url"
								class={fieldClasses}
								placeholder="Leave empty to use the one in the spec"
								autocomplete="off"
								aria-invalid={errors.baseUrl ? 'true' : undefined}
							/>
						</Field>
					</div>
				{/if}

				{#if mode === 'new' && kind === 'mcp-stdio'}
					<div>
						<Field
							label="Command"
							id={`${formId}-command`}
							error={errors.command}
							hint="Arguments are split on spaces; quotes are respected."
						>
							<input
								id={`${formId}-command`}
								bind:value={command}
								type="text"
								class={fieldClasses}
								placeholder="npx -y @modelcontextprotocol/server-github"
								autocomplete="off"
								spellcheck="false"
								aria-invalid={errors.command ? 'true' : undefined}
							/>
						</Field>

						{#if parsedCommand.length > 0}
							<div class="mt-2 flex flex-wrap gap-1.5">
								{#each parsedCommand as argument, index (index)}
									<code
										class="rounded bg-zinc-100 px-1.5 py-0.5 text-xs text-zinc-600 dark:bg-zinc-800 dark:text-zinc-300"
									>
										{argument}
									</code>
								{/each}
							</div>
						{/if}
					</div>
				{/if}

				{#if mode === 'new' && kind === 'command'}
					<div
						class="rounded-lg border border-zinc-200 bg-zinc-50 p-3 text-sm text-zinc-600 dark:border-zinc-800 dark:bg-zinc-950/60 dark:text-zinc-400"
					>
						You can add tools to this service after creating it.
					</div>
				{/if}

				{#if mode === 'existing' && selectedService?.auth === 'oauth'}
					<div
						class="rounded-lg border border-zinc-200 bg-zinc-50 p-3 text-sm text-zinc-600 dark:border-zinc-800 dark:bg-zinc-950/60 dark:text-zinc-400"
					>
						This service uses OAuth. You'll sign in after creating the account.
					</div>
				{/if}

				{#if showHeaders}
					<div>
						<div class="flex items-center justify-between gap-3">
							<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">Headers</p>
							<Button variant="secondary" icon="plus" class="px-3" onclick={addHeader}>
								Add header
							</Button>
						</div>

						{#if headerRows.length === 0}
							<p class="mt-2 text-sm text-zinc-500">No headers yet.</p>
						{/if}

						<div class="mt-2 space-y-2">
							{#each headerRows as row (row.id)}
								<div class="grid gap-2 sm:grid-cols-[1fr_1fr_44px]">
									<input
										bind:value={row.name}
										type="text"
										class={fieldClasses}
										placeholder="Authorization"
										aria-label="Header name"
										autocomplete="off"
										spellcheck="false"
									/>
									<input
										bind:value={row.value}
										type="password"
										class={fieldClasses}
										placeholder="Bearer ..."
										aria-label="Header value"
										autocomplete="off"
									/>
									<button
										type="button"
										class="inline-flex size-11 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
										aria-label="Remove header"
										onclick={() => removeHeader(row.id)}
									>
										<Icon name="x" class="size-5" />
									</button>
								</div>
							{/each}
						</div>

						{#if errors.headers}
							<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.headers}</p>
						{/if}
					</div>
				{/if}

				{#if showEnv}
					<div>
						<div class="flex items-center justify-between gap-3">
							<p class="text-sm font-medium text-zinc-700 dark:text-zinc-300">
								Environment variables
							</p>
							<Button variant="secondary" icon="plus" class="px-3" onclick={addEnvironmentVariable}>
								Add variable
							</Button>
						</div>

						{#if envRows.length === 0}
							<p class="mt-2 text-sm text-zinc-500">No environment variables yet.</p>
						{/if}

						<div class="mt-2 space-y-2">
							{#each envRows as row (row.id)}
								<div class="rounded-lg border border-zinc-200 p-2 dark:border-zinc-700">
									<div class="grid gap-2 sm:grid-cols-[1fr_1fr_44px]">
										<input
											bind:value={row.key}
											type="text"
											class={fieldClasses}
											placeholder="GITHUB_TOKEN"
											aria-label="Variable name"
											autocomplete="off"
											spellcheck="false"
										/>
										<input
											bind:value={row.value}
											type={row.secret ? 'password' : 'text'}
											class={fieldClasses}
											placeholder="Value"
											aria-label="Variable value"
											autocomplete="off"
										/>
										<button
											type="button"
											class="inline-flex size-11 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
											aria-label="Remove variable"
											onclick={() => removeEnvironmentVariable(row.id)}
										>
											<Icon name="x" class="size-5" />
										</button>
									</div>

									<div class="mt-1 flex items-center justify-end gap-2">
										<span class="text-xs text-zinc-500">Secret</span>
										<button
											type="button"
											role="switch"
											aria-checked={row.secret}
											aria-label="Secret"
											class="flex h-11 w-14 items-center justify-center rounded-lg focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none"
											onclick={() => toggleSecret(row)}
										>
											<span
												class={[
													'relative h-6 w-11 rounded-full transition-colors',
													row.secret
														? 'bg-zinc-900 dark:bg-zinc-100'
														: 'bg-zinc-300 dark:bg-zinc-700'
												].join(' ')}
											>
												<span
													class={[
														'absolute top-0.5 left-0.5 size-5 rounded-full bg-white shadow transition-transform',
														row.secret ? 'translate-x-5' : ''
													].join(' ')}
												></span>
											</span>
										</button>
									</div>
								</div>
							{/each}
						</div>

						{#if errors.env}
							<p class="mt-1.5 text-xs text-red-600 dark:text-red-400">{errors.env}</p>
						{/if}
					</div>
				{/if}

				{#if mode === 'new'}
					<div class="space-y-4">
						<Field
							label="Service name"
							id={`${formId}-service-name`}
							error={errors.serviceName}
							hint="Lowercase letters, digits and dashes."
						>
							<input
								id={`${formId}-service-name`}
								value={serviceName}
								type="text"
								class={fieldClasses}
								placeholder="todoist"
								autocomplete="off"
								aria-invalid={errors.serviceName ? 'true' : undefined}
								oninput={(event) => {
									serviceName = event.currentTarget.value;
									serviceNameTouched = true;
								}}
							/>
						</Field>

						<Field
							label="Service description"
							id={`${formId}-service-description`}
							hint="Optional."
						>
							<input
								id={`${formId}-service-description`}
								bind:value={serviceDescription}
								type="text"
								class={fieldClasses}
								placeholder="e.g. Personal and team tasks"
								autocomplete="off"
							/>
						</Field>
					</div>
					{@render accountFields()}
				{/if}
			</fieldset>

			{#if apiError}
				<div
					class="rounded-lg border border-red-200 bg-red-50 p-3 text-sm break-words text-red-700 dark:border-red-500/30 dark:bg-red-500/10 dark:text-red-300"
				>
					{apiError}
				</div>
			{/if}
		</form>
	{/if}

	{#snippet footer()}
		{#if result && resultStatus}
			{#if showResultLogin}
				<Button variant="secondary" onclick={onclose} disabled={loginWaiting}>Later</Button>
				<Button loading={loginWaiting} onclick={loginResult}>Log in now</Button>
			{:else}
				<Button onclick={onclose}>Done</Button>
			{/if}
		{:else}
			<Button variant="secondary" onclick={onclose} disabled={submitting}>Cancel</Button>
			<Button type="submit" form={formId} loading={submitting}>Add account</Button>
		{/if}
	{/snippet}
</Panel>
