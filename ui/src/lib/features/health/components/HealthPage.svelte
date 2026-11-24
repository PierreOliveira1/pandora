<script lang="ts">
	import { onMount } from 'svelte';
	import { errorMessage } from '#lib/shared/api/client.ts';
	import { Button, Icon, Skeleton, Spinner, Switch, showToast } from '#lib/shared/ui/index.ts';
	import { loadAutostart, loadChecks, saveAutostart } from '../api.ts';
	import type { AutostartInfo, DoctorCheck } from '../types.ts';

	let checks = $state<DoctorCheck[]>([]);
	let loading = $state(true);
	let running = $state(false);
	let fullLoading = $state(false);
	let error = $state('');
	let fullError = $state('');
	let copied = $state<string | null>(null);
	let autostart = $state<AutostartInfo | null>(null);
	let autostartEnabled = $state(false);
	let autostartSaving = $state(false);
	let autostartError = $state('');

	const problemCount = $derived(checks.filter((check) => !check.ok).length);

	onMount(() => {
		void runChecks();
		void loadAutostartSettings();
	});

	async function runChecks(): Promise<void> {
		if (running) return;

		running = true;
		loading = true;
		error = '';
		fullError = '';
		copied = null;

		try {
			checks = await loadChecks(true);
			loading = false;
			fullLoading = true;

			try {
				checks = await loadChecks(false);
			} catch (cause) {
				fullError = errorMessage(cause);
			} finally {
				fullLoading = false;
			}
		} catch (cause) {
			error = errorMessage(cause);
			loading = false;
		} finally {
			running = false;
		}
	}

	async function copyFix(name: string, fix: string): Promise<void> {
		try {
			await navigator.clipboard.writeText(fix);
			copied = name;
			showToast('Copied to clipboard.');
		} catch {
			showToast('Could not copy the fix to the clipboard.', 'error');
		}
	}

	async function loadAutostartSettings(): Promise<void> {
		try {
			const info = await loadAutostart();
			autostart = info;
			autostartEnabled = info.enabled;
		} catch {
			autostart = null;
		}
	}

	async function updateAutostart(enabled: boolean): Promise<void> {
		if (!autostart?.supported || autostartSaving) return;

		const previous = autostart.enabled;
		autostartSaving = true;
		autostartError = '';
		autostartEnabled = enabled;

		try {
			const info = await saveAutostart(enabled);
			autostart = info;
			autostartEnabled = info.enabled;
			showToast(
				info.enabled ? 'Pandora now starts at login.' : 'Pandora no longer starts at login.'
			);
		} catch (cause) {
			autostartEnabled = previous;
			autostart = { ...autostart, enabled: previous };
			autostartError = errorMessage(cause);
		} finally {
			autostartSaving = false;
		}
	}

	function handleAutostartChange(event: Event): void {
		const target = event.target;
		if (!(target instanceof HTMLInputElement) || target.type !== 'checkbox') return;
		autostartEnabled = target.checked;
		void updateAutostart(target.checked);
	}
</script>

<div>
	<div class="flex flex-wrap items-start justify-between gap-3">
		<div>
			<h1 class="text-2xl font-semibold tracking-tight">Health</h1>
		</div>
		<Button variant="secondary" icon="refresh" loading={running} onclick={() => void runChecks()}>
			Run checks again
		</Button>
	</div>

	{#if fullLoading}
		<div class="mt-3 flex items-center gap-2 text-sm text-zinc-500">
			<Spinner />
			<span>Checking connections...</span>
		</div>
	{/if}

	{#if fullError}
		<div
			class="mt-4 rounded-lg border border-amber-200 bg-amber-50 p-3 text-sm break-words text-amber-800 dark:border-amber-500/30 dark:bg-amber-500/10 dark:text-amber-300"
		>
			Full connection checks failed: {fullError}
		</div>
	{/if}

	{#if loading}
		<div
			class="mt-6 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			{#each [0, 1, 2, 3] as row (row)}
				<div
					class="flex items-start gap-3 border-b border-zinc-100 py-3 last:border-0 dark:border-zinc-800"
				>
					<Skeleton class="size-5 rounded-full" />
					<div class="min-w-0 flex-1">
						<Skeleton class="h-4 w-40" />
						<Skeleton class="mt-2 h-3 w-64" />
					</div>
				</div>
			{/each}
		</div>
	{:else if error}
		<div
			class="mt-6 rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-xl font-semibold">Could not run checks</h2>
			<p class="mt-2 text-sm break-words text-zinc-500">{error}</p>
			<Button variant="secondary" icon="refresh" class="mt-4" onclick={() => void runChecks()}>
				Try again
			</Button>
		</div>
	{:else if checks.length === 0}
		<div
			class="mt-6 rounded-xl border border-zinc-200 bg-white px-6 py-12 text-center text-sm text-zinc-500 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			No checks returned.
		</div>
	{:else}
		<div class="mt-6 flex items-center gap-2">
			{#if problemCount === 0}
				<span
					class="flex size-7 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400"
				>
					<Icon name="check" class="size-4" />
				</span>
				<p class="text-xl font-semibold text-emerald-600 dark:text-emerald-400">All good</p>
			{:else}
				<span
					class="flex size-7 items-center justify-center rounded-full bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-400"
				>
					<Icon name="alert" class="size-4" />
				</span>
				<p class="text-xl font-semibold text-red-600 dark:text-red-400">
					{problemCount}
					{problemCount === 1 ? 'problem' : 'problems'}
				</p>
			{/if}
		</div>

		<div
			class="mt-4 overflow-hidden rounded-xl border border-zinc-200 bg-white shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<ul>
				{#each checks as check (check.name)}
					<li class="border-b border-zinc-200 p-4 last:border-0 dark:border-zinc-800">
						<div class="flex items-start gap-3">
							{#if check.ok}
								<span
									class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-emerald-50 text-emerald-600 dark:bg-emerald-500/15 dark:text-emerald-400"
								>
									<Icon name="check" class="size-4" />
								</span>
							{:else}
								<span
									class="mt-0.5 flex size-6 shrink-0 items-center justify-center rounded-full bg-red-50 text-red-600 dark:bg-red-500/15 dark:text-red-400"
								>
									<Icon name="alert" class="size-4" />
								</span>
							{/if}

							<div class="min-w-0 flex-1">
								<p class="text-sm font-medium text-zinc-800 dark:text-zinc-100">{check.name}</p>
								{#if check.info}
									<p class="mt-0.5 text-sm break-words text-zinc-500">{check.info}</p>
								{/if}

								{#if check.fix}
									<div class="mt-2 flex flex-col gap-2 sm:flex-row sm:items-start">
										<pre
											class="flex min-h-11 min-w-0 flex-1 items-center overflow-x-auto rounded-lg bg-zinc-100 px-3 py-2 font-mono text-xs break-words whitespace-pre-wrap text-zinc-700 dark:bg-zinc-950 dark:text-zinc-300">{check.fix}</pre>
										<Button
											variant="secondary"
											class="self-start"
											onclick={() => void copyFix(check.name, check.fix ?? '')}
										>
											{copied === check.name ? 'Copied' : 'Copy'}
										</Button>
									</div>
								{/if}
							</div>
						</div>
					</li>
				{/each}
			</ul>
		</div>
	{/if}

	{#if !loading && autostart?.supported}
		<section
			class="mt-4 rounded-xl border border-zinc-200 bg-white p-4 shadow-sm sm:p-5 dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h2 class="text-base font-semibold">Background</h2>
			<div class="mt-2" onchange={handleAutostartChange}>
				<Switch
					bind:checked={autostartEnabled}
					label="Start Pandora at login"
					description="Keeps webhooks receiving events after a restart. Your computer must be on and awake."
					disabled={autostartSaving}
				/>
			</div>
			{#if autostartError}
				<p class="mt-2 text-sm text-red-600 dark:text-red-400">{autostartError}</p>
			{/if}
		</section>
	{/if}
</div>

<svelte:head>
	<title>Health · Pandora</title>
</svelte:head>
