<script lang="ts">
	import { RulesPanel } from '#lib/features/accounts/index.ts';
	import { ToolsPage } from '#lib/features/tools/index.ts';

	let rulesRef = $state<string | null>(null);
</script>

<ToolsPage onOpenRules={(ref) => (rulesRef = ref)} />

{#if rulesRef}
	<RulesPanel accountRef={rulesRef} onclose={() => (rulesRef = null)} />
{/if}
