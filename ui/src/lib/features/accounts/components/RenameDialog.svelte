<script lang="ts">
	import { untrack } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Dialog, Field, fieldClasses } from '#lib/shared/ui/index.ts';
	import { validateName } from '../schemas.ts';

	let {
		title,
		initialValue,
		onclose,
		onsave
	}: {
		title: string;
		initialValue: string;
		onclose: () => void;
		onsave: (name: string) => Promise<string | null>;
	} = $props();

	const baseId = $props.id();
	const fieldId = `${baseId}-name`;
	const formId = `${baseId}-form`;

	let value = $state(untrack(() => initialValue));
	let error = $state('');
	let submitting = $state(false);
	let input = $state<HTMLInputElement | null>(null);

	$effect(() => {
		input?.focus();
		input?.select();
	});

	function handleInput(): void {
		if (error) error = validateName(value) ?? '';
	}

	async function submit(event: SubmitEvent): Promise<void> {
		event.preventDefault();
		const validation = validateName(value);
		if (validation) {
			error = validation;
			return;
		}

		submitting = true;
		try {
			const message = await onsave(value.trim());
			if (message) error = message;
		} catch (cause) {
			error = errorMessage(cause);
		} finally {
			submitting = false;
		}
	}
</script>

<Dialog {title} {onclose}>
	<form id={formId} onsubmit={submit} class="space-y-4">
		<Field label="New name" id={fieldId} {error}>
			<input
				bind:this={input}
				bind:value
				id={fieldId}
				type="text"
				class={fieldClasses}
				autocomplete="off"
				aria-invalid={error ? 'true' : undefined}
				oninput={handleInput}
				disabled={submitting}
			/>
		</Field>
	</form>

	{#snippet footer()}
		<Button variant="secondary" onclick={onclose} disabled={submitting}>Cancel</Button>
		<Button type="submit" form={formId} loading={submitting}>Save</Button>
	{/snippet}
</Dialog>
