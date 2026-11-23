import { api } from '#lib/shared/api/client.ts';
import type {
	CallToolResult,
	CommandToolInput,
	JsonValue,
	Overview,
	ToolCatalog
} from './types.ts';

export function loadOverview(): Promise<Overview> {
	return api.getOverview();
}

export function loadTools(service: string, refresh = false): Promise<ToolCatalog> {
	return api.getTools(service, refresh);
}

export function callTool(
	ref: string,
	tool: string,
	args: Record<string, JsonValue>
): Promise<CallToolResult> {
	return api.callTool({ ref, tool, args });
}

export function addCommandTool(service: string, input: CommandToolInput): Promise<void> {
	return api.addCommandTool(service, input);
}

export function removeCommandTool(service: string, name: string): Promise<void> {
	return api.removeCommandTool(service, name);
}
