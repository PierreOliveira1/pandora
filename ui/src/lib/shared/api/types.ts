export type ServiceKind = 'mcp-http' | 'mcp-stdio' | 'openapi' | 'command';

export type AuthMode = 'none' | 'header' | 'oauth';

export type ExposeMode = 'compact' | 'direct';

export type JsonValue =
	string | number | boolean | null | JsonValue[] | { [key: string]: JsonValue };

export type JsonSchema = {
	type?: string | string[];
	description?: string;
	properties?: Record<string, JsonSchema>;
	required?: string[];
	items?: JsonSchema;
	enum?: JsonValue[];
	default?: JsonValue;
};

export type ToolAnnotations = {
	readOnlyHint?: boolean;
	destructiveHint?: boolean;
};

export type ToolInfo = {
	name: string;
	description?: string;
	inputSchema: JsonSchema;
	annotations?: ToolAnnotations;
};

export type ToolCatalog = {
	fetched_at: string;
	instructions?: string;
	tools: ToolInfo[];
	warning?: string;
};

export type ToolContent = {
	type: string;
	text?: string;
	data?: string;
	mimeType?: string;
};

export type CallToolResult = {
	content: ToolContent[];
	isError?: boolean;
	structuredContent?: JsonValue;
};

export type CallToolInput = {
	ref: string;
	tool: string;
	args: Record<string, JsonValue>;
};

export type AccountPolicy = {
	readonly: boolean;
	allow: string[];
	deny: string[];
};

export type CommandParamType = 'string' | 'number' | 'integer' | 'boolean' | 'array';

export type CommandParam = {
	type: CommandParamType;
	description?: string;
	required?: boolean;
};

export type CommandToolView = {
	name: string;
	description?: string;
	command: string[];
	params?: Record<string, CommandParam>;
	readonly?: boolean;
	timeout_sec?: number;
};

export type CommandToolInput = {
	name: string;
	description?: string;
	command: string[];
	params?: Record<string, CommandParam>;
	readonly?: boolean;
	timeout_sec?: number;
};

export type ClientId = 'claude-code' | 'codex' | 'opencode' | 'cursor' | 'claude-desktop';

export type IntegrationMode = '' | 'skill' | 'mcp';

export type IntegrationView = {
	client: ClientId;
	detected: boolean;
	supports_skill: boolean;
	mode: IntegrationMode;
};

export type InstallReport = {
	client: ClientId;
	mode: string;
	skill_dir?: string;
	mcp_config?: string;
	permission_file?: string;
	permission_note?: string;
};

export type DoctorCheck = {
	name: string;
	ok: boolean;
	info?: string;
	fix?: string;
};

export type LogEntry = {
	time: string;
	ref: string;
	tool: string;
	client: string;
	duration_ms: number;
	ok: boolean;
	blocked?: boolean;
	error?: string;
};

export type WebhookInfo = {
	ref: string;
	style: string;
	challenge?: string;
	url?: string;
	local_url?: string;
	last_event_at?: string;
};

export type EventView = {
	seq: number;
	id: string;
	received_at: string;
	service: string;
	account: string;
	topic?: string;
	headers?: Record<string, string>;
	payload: JsonValue;
};

export type AutostartInfo = {
	supported: boolean;
	enabled: boolean;
};

export type AccountView = {
	name: string;
	description?: string;
	default: boolean;
	readonly: boolean;
	allow: string[];
	deny: string[];
};

export type ServiceView = {
	name: string;
	kind: ServiceKind;
	description?: string;
	auth: AuthMode;
	expose: ExposeMode;
	url?: string;
	command?: string[];
	spec?: string;
	default_account?: string;
	tools_known: boolean;
	tool_count: number;
	command_tools?: CommandToolView[];
	accounts: AccountView[];
};

export type Overview = {
	version: string;
	services: ServiceView[];
	integrations: string[];
};

export type StatusInfo = {
	ref: string;
	ok: boolean;
	needs_login: boolean;
	tools: number;
	latency_ms: number;
	error?: string;
};

export type AddAccountInput = {
	service: string;
	account: string;
	kind?: ServiceKind;
	url?: string;
	command?: string[];
	spec?: string;
	auth?: 'oauth';
	oauth_client_id?: string;
	oauth_client_secret?: string;
	scopes?: string[];
	service_description?: string;
	description?: string;
	env?: Record<string, string>;
	secret_env?: Record<string, string>;
	secret_headers?: Record<string, string>;
};

export type AddAccountResponse = {
	ref: string;
	status: StatusInfo;
	warning?: string;
};

export type LoginState = 'pending' | 'done' | 'error';

export type LoginResult = {
	state: LoginState;
	error?: string;
};
