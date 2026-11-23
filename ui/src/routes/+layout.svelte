<script lang="ts">
	import '../app.css';
	import { onMount, type Snippet } from 'svelte';
	import { page } from '$app/state';
	import { api } from '#lib/shared/api/client.ts';
	import { session } from '#lib/shared/api/session.svelte.ts';
	import { Icon, Toast } from '#lib/shared/ui/index.ts';

	let { children }: { children: Snippet } = $props();

	const navItems = [
		{ href: '/', label: 'Accounts' },
		{ href: '/tools', label: 'Tools' },
		{ href: '/activity', label: 'Activity' },
		{ href: '/ai-clients', label: 'AI clients' },
		{ href: '/health', label: 'Health' }
	];

	let drawerOpen = $state(false);
	let version = $state('');

	const pathname = $derived(page.url.pathname);

	onMount(async () => {
		try {
			const overview = await api.getOverview();
			version = overview.version;
		} catch {
			version = '';
		}
	});

	function isActive(href: string): boolean {
		return href === '/' ? pathname === '/' : pathname === href || pathname.startsWith(`${href}/`);
	}

	function closeDrawer(): void {
		drawerOpen = false;
	}
</script>

{#snippet navigation(onNavigate: () => void)}
	<ul class="flex flex-col gap-1">
		{#each navItems as item (item.href)}
			{@const active = isActive(item.href)}
			<li>
				<a
					href={item.href}
					aria-current={active ? 'page' : undefined}
					onclick={onNavigate}
					class={[
						'flex min-h-11 items-center rounded-lg px-3 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none',
						active
							? 'bg-violet-50 text-violet-600 dark:bg-violet-500/15 dark:text-violet-300'
							: 'text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100'
					].join(' ')}
				>
					{item.label}
				</a>
			</li>
		{/each}
	</ul>
{/snippet}

<svelte:head>
	<title>Pandora</title>
</svelte:head>

{#if session.expired}
	<main class="flex min-h-screen items-center justify-center p-4">
		<div
			class="w-full max-w-md rounded-xl border border-zinc-200 bg-white p-6 shadow-sm dark:border-zinc-800 dark:bg-zinc-900"
		>
			<h1 class="text-xl font-semibold">Session expired</h1>
			<p class="mt-2 text-sm text-zinc-500">
				Open the link printed by <code class="font-mono text-zinc-700 dark:text-zinc-300">
					pandora ui
				</code>.
			</p>
		</div>
	</main>
{:else}
	<div class="min-h-screen">
		<aside
			class="hidden border-r border-zinc-200 bg-white lg:fixed lg:inset-y-0 lg:left-0 lg:z-40 lg:flex lg:w-60 lg:flex-col dark:border-zinc-800 dark:bg-zinc-900"
		>
			<div class="flex min-h-16 items-center gap-2 px-5">
				<img src="/favicon.svg" alt="" class="size-7 shrink-0" />
				<span class="text-base font-semibold">Pandora</span>
			</div>

			<nav class="flex-1 px-3 py-3" aria-label="Main">
				{@render navigation(() => {})}
			</nav>

			<div class="min-h-12 px-5 py-4 text-xs text-zinc-500">
				{#if version}
					v{version}
				{/if}
			</div>
		</aside>

		{#if drawerOpen}
			<button
				type="button"
				class="fixed inset-0 z-40 h-full w-full bg-zinc-950/50 lg:hidden"
				aria-label="Close menu"
				onclick={closeDrawer}
			></button>

			<aside
				class="fixed inset-y-0 left-0 z-50 flex w-60 flex-col border-r border-zinc-200 bg-white shadow-xl lg:hidden dark:border-zinc-800 dark:bg-zinc-900"
			>
				<div class="flex min-h-16 items-center justify-between gap-2 px-3 pl-5">
					<div class="flex items-center gap-2">
						<img src="/favicon.svg" alt="" class="size-7 shrink-0" />
						<span class="text-base font-semibold">Pandora</span>
					</div>
					<button
						type="button"
						class="inline-flex size-11 items-center justify-center rounded-lg text-zinc-500 hover:bg-zinc-100 hover:text-zinc-800 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-400 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
						aria-label="Close menu"
						onclick={closeDrawer}
					>
						<Icon name="x" class="size-5" />
					</button>
				</div>

				<nav class="flex-1 px-3 py-3" aria-label="Main">
					{@render navigation(closeDrawer)}
				</nav>

				<div class="min-h-12 px-5 py-4 text-xs text-zinc-500">
					{#if version}
						v{version}
					{/if}
				</div>
			</aside>
		{/if}

		<header
			class="fixed inset-x-0 top-0 z-30 flex h-14 items-center gap-2 border-b border-zinc-200 bg-white px-2 lg:hidden dark:border-zinc-800 dark:bg-zinc-900"
		>
			<button
				type="button"
				class="inline-flex size-11 items-center justify-center rounded-lg text-zinc-600 hover:bg-zinc-100 hover:text-zinc-900 focus-visible:ring-2 focus-visible:ring-violet-500 focus-visible:outline-none dark:text-zinc-300 dark:hover:bg-zinc-800 dark:hover:text-zinc-100"
				aria-label="Open menu"
				aria-expanded={drawerOpen}
				onclick={() => (drawerOpen = true)}
			>
				<Icon name="menu" class="size-5" />
			</button>
			<div class="flex items-center gap-2">
				<img src="/favicon.svg" alt="" class="size-6 shrink-0" />
				<span class="font-semibold">Pandora</span>
			</div>
		</header>

		<div class="pt-14 lg:pt-0 lg:pl-60">
			<main class="mx-auto w-full max-w-[960px] px-4 py-6 lg:px-8 lg:py-8">
				{@render children()}
			</main>
		</div>
	</div>
{/if}

<Toast />
