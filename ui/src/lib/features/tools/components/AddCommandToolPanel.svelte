<script lang="ts">
	import { tick } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
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
	import { parseCommandLine } from '#lib/shared/utils/parse-command.ts';
	import type { CommandParam, CommandParamType, ServiceView } from '../types.ts';

	type ParameterRow = {
		id: number;
		name: string;
		type: CommandParamType;
		required: boolean;
		description: string;
	};

	const typeOptions: Array<{ value: CommandParamType; label: string }> = [
		{ value: 'string', label: 'string' },
		{ value: 'number', label: 'number' },
		{ value: 'integer', label: 'integer' },
		{ value: 'boolean', label: 'boolean' },
		{ value: 'array', label: 'array' }
	];

	const commandHint = 'Use {{parameter}} placeholders, for example ./deploy.sh {{environment}}.';
	const commandPlaceholder = './scripts/deploy.sh {{environment}}';

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
	let name = $state('');
	let description = $state('');
	let command = $state('');
	let parameters = $state<ParameterRow[]>([]);
	let readonly = $state(false);
	let timeout = $state('60');
	let errors = $state<Record<string, string>>({});
	let apiError = $state('');
	let submitting = $state(false);
	let panelBody = $state<HTMLElement | null>(null);
	let rowSequence = 0;

	const parsedCommand = $derived(parseCommandLine(command));
	const placeholders = $derived.by(() => {
		const names = new Set<string>();
		const pattern = /\{\{\s*([a-zA-Z0-9_-]+)\s*\}\}/g;
		let match = pattern.exec(command);

		while (match) {
			if (match[1]) names.add(match[1]);
			match = pattern.exec(command);
		}

		return [...names];
	});
	const parameterNames = $derived(
		parameters.map((parameter) => parameter.name.trim()).filter(Boolean)
	);
	const missingPlaceholders = $derived(
		placeholders.filter((placeholder) => !parameterNames.includes(placeholder))
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

	function validate(): Record<string, string> {
		const next: Record<string, string> = {};

		if (!name.trim()) next.name = 'Enter a tool name.';
		if (!command.trim()) {
			next.command = 'Enter the command to run.';
		} else if (parsedCommand.length === 0 || !parsedCommand[0]) {
			next.command = 'Enter at least one command argument.';
		}

		const timeoutValue = Number(timeout);
		if (!timeout.trim() || !Number.isInteger(timeoutValue) || timeoutValue < 1) {
			next.timeout = 'Enter a timeout in seconds.';
		}

		const seen = new Set<string>();
		for (const parameter of parameters) {
			const parameterName = parameter.name.trim();
			if (!parameterName) {
				if (parameter.description.trim() || parameter.required) {
					next.parameters = 'Enter a name for each parameter.';
				}
				continue;
			}
			if (seen.has(parameterName)) {
				next.parameters = 'Parameter names must be unique.';
			}
			seen.add(parameterName);
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
			await addCommandTool(service.name, {
				name: name.trim(),
				description: description.trim() || undefined,
				command: parsedCommand,
				params: paramsPayload(),
				readonly,
				timeout_sec: Number(timeout)
			});
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

<Panel title="Add command tool" {onclose} bind:body={panelBody}>
	<form id={formId} onsubmit={submit} class="space-y-6" novalidate>
		<fieldset disabled={submitting} class="min-w-0 space-y-6 border-0 p-0">
			<Field label="Name" id={`${formId}-name`} error={errors.name}>
				<input
					id={`${formId}-name`}
					bind:value={name}
					type="text"
					class={fieldClasses}
					placeholder="deploy"
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
					placeholder="Deploy the app to an environment"
					autocomplete="off"
				/>
			</Field>

			<div>
				<Field label="Command" id={`${formId}-command`} error={errors.command} hint={commandHint}>
					<input
						id={`${formId}-command`}
						bind:value={command}
						type="text"
						class={fieldClasses}
						placeholder={commandPlaceholder}
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

				{#if missingPlaceholders.length > 0}
					<div
						class="mt-2 rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
					>
						No parameter for
						{#each missingPlaceholders as placeholder, index (placeholder)}
							<code class="font-mono">{`{{${placeholder}}}`}</code>{index <
							missingPlaceholders.length - 1
								? ', '
								: '.'}
						{/each}
					</div>
				{/if}
			</div>

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
										placeholder="environment"
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

			<div class="space-y-4">
				<Switch bind:checked={readonly} label="Read-only" />
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
