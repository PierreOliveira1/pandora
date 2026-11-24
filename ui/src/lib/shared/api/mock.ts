import { ApiError, type HttpMethod } from './client.ts';
import type {
	AccountPolicy,
	AccountView,
	AddAccountInput,
	AuthMode,
	AutostartInfo,
	CallToolInput,
	CallToolResult,
	CommandParam,
	CommandToolInput,
	CommandToolView,
	DoctorCheck,
	EventView,
	InstallReport,
	IntegrationView,
	JsonSchema,
	JsonValue,
	LoginResult,
	LogEntry,
	Overview,
	ServiceKind,
	ServiceView,
	StatusInfo,
	ToolCatalog,
	ToolContent,
	ToolInfo,
	WebhookInfo
} from './types.ts';

const MOCK_VERSION = '0.1.0';
const MOCK_INTEGRATIONS = ['Claude Code', 'Cursor', 'Codex'];
const NAME_PATTERN = /^[a-z0-9][a-z0-9-]{0,31}$/;

type MockLogin = {
	ref: string;
	startedAt: number;
};

function delay(ms: number): Promise<void> {
	return new Promise((resolve) => setTimeout(resolve, ms));
}

function seedServices(): ServiceView[] {
	return [
		{
			name: 'todoist',
			kind: 'mcp-http',
			description: 'Personal and team tasks.',
			auth: 'oauth',
			expose: 'compact',
			url: 'https://mcp.todoist.com/mcp',
			default_account: 'personal',
			tools_known: true,
			tool_count: 3,
			accounts: [
				{
					name: 'personal',
					description: 'Tasks for my own projects.',
					default: true,
					readonly: false,
					allow: [],
					deny: []
				},
				{
					name: 'work',
					description: 'Shared projects with the team.',
					default: false,
					readonly: false,
					allow: [],
					deny: []
				}
			]
		},
		{
			name: 'meta',
			kind: 'mcp-http',
			description: 'Facebook and Instagram business tools.',
			auth: 'oauth',
			expose: 'compact',
			url: 'https://mcp.meta.com/mcp',
			default_account: 'clothing-store',
			tools_known: true,
			tool_count: 4,
			accounts: [
				{
					name: 'clothing-store',
					description: 'Clothing store Page and Instagram.',
					default: true,
					readonly: true,
					allow: [],
					deny: []
				},
				{
					name: 'shoe-store',
					description: 'Footwear store Page.',
					default: false,
					readonly: false,
					allow: ['get_*'],
					deny: []
				}
			]
		},
		{
			name: 'instagram',
			kind: 'openapi',
			description: 'Instagram comments, messages and publishing.',
			auth: 'oauth',
			expose: 'compact',
			url: 'https://graph.instagram.com',
			spec: 'https://graph.instagram.com/openapi.json',
			default_account: 'store',
			tools_known: true,
			tool_count: 2,
			accounts: [
				{
					name: 'store',
					description: 'The Trono Instagram account.',
					default: true,
					readonly: false,
					allow: [],
					deny: []
				}
			]
		},
		{
			name: 'github',
			kind: 'mcp-stdio',
			description: 'Repositories, issues and pull requests.',
			auth: 'none',
			expose: 'compact',
			command: ['npx', '-y', '@modelcontextprotocol/server-github'],
			default_account: 'company',
			tools_known: true,
			tool_count: 2,
			accounts: [
				{
					name: 'company',
					description: 'The company GitHub organization.',
					default: true,
					readonly: false,
					allow: [],
					deny: ['delete_*']
				},
				{
					name: 'work',
					description: 'Work repositories.',
					default: false,
					readonly: false,
					allow: [],
					deny: []
				}
			]
		},
		{
			name: 'billing-api',
			kind: 'openapi',
			description: 'Invoices and subscriptions.',
			auth: 'header',
			expose: 'compact',
			url: 'https://billing.example.com/v1',
			spec: 'https://billing.example.com/openapi.json',
			default_account: 'prod',
			tools_known: true,
			tool_count: 2,
			accounts: [
				{
					name: 'prod',
					description: 'Production billing workspace.',
					default: true,
					readonly: false,
					allow: [],
					deny: []
				}
			]
		},
		{
			name: 'ops',
			kind: 'command',
			description: 'Local maintenance commands.',
			auth: 'none',
			expose: 'compact',
			default_account: 'local',
			tools_known: true,
			tool_count: 2,
			command_tools: [
				{
					name: 'disk-usage',
					description: 'Show disk usage for the workspace.',
					command: ['df', '-h'],
					readonly: true,
					timeout_sec: 30
				},
				{
					name: 'deploy',
					description: 'Deploy the app to an environment.',
					command: ['./scripts/deploy.sh', '{{environment}}'],
					params: {
						environment: {
							type: 'string',
							description: 'Environment to deploy to.',
							required: true
						}
					},
					readonly: false,
					timeout_sec: 120
				}
			],
			accounts: [
				{
					name: 'local',
					description: 'Commands on this computer.',
					default: true,
					readonly: false,
					allow: [],
					deny: []
				}
			]
		},
		{
			name: 'shop-api',
			kind: 'http',
			description: 'Store orders API, called directly over HTTP.',
			auth: 'none',
			expose: 'compact',
			urls: { api: 'https://shop.example.com/v1' },
			headers: { Authorization: 'Bearer {{env.SHOP_TOKEN}}' },
			default_account: 'store',
			tools_known: true,
			tool_count: 3,
			command_tools: [
				{
					name: 'cancel-order',
					description: 'Cancel an order by id.',
					http: {
						url: 'api',
						method: 'POST',
						path: '/stores/{{env.STORE_ID}}/orders/{{id}}/cancel',
						json: { reason: '{{reason}}' }
					},
					params: {
						id: { type: 'string', required: true, description: 'Order id.' },
						reason: { type: 'string', description: 'Why the order is cancelled.' }
					},
					readonly: false,
					timeout_sec: 60
				},
				{
					name: 'list-orders',
					description: 'List the most recent orders.',
					http: {
						url: 'api',
						method: 'GET',
						path: '/stores/{{env.STORE_ID}}/orders',
						query: { limit: '{{limit}}', fields: 'id,total,status' }
					},
					params: { limit: { type: 'integer', description: 'How many orders to return.' } },
					readonly: true,
					timeout_sec: 60
				}
			],
			accounts: [
				{
					name: 'store',
					description: 'Main store.',
					default: true,
					readonly: false,
					allow: [],
					deny: [],
					env: { STORE_ID: '42' },
					secret_env: ['SHOP_TOKEN'],
					secret_headers: []
				}
			]
		}
	];
}

function refOf(service: string, account: string): string {
	return `${service}/${account}`;
}

function makeTool(
	name: string,
	description: string,
	properties: Record<string, JsonSchema>,
	required: string[] = [],
	annotations?: ToolInfo['annotations']
): ToolInfo {
	return {
		name,
		description,
		inputSchema: { type: 'object', properties, required },
		annotations
	};
}

function commandToolInfo(tool: CommandToolView): ToolInfo {
	const properties: Record<string, JsonSchema> = {};
	const required: string[] = [];

	for (const [name, param] of Object.entries(tool.params ?? {})) {
		properties[name] = {
			type: param.type,
			description: param.description
		};
		if (param.required) required.push(name);
	}

	return {
		name: tool.name,
		description: tool.description,
		inputSchema: { type: 'object', properties, required },
		annotations: { readOnlyHint: Boolean(tool.readonly) }
	};
}

function seedCatalogs(): Map<string, ToolCatalog> {
	const fetchedAt = new Date(Date.now() - 5 * 60_000).toISOString();
	const catalogs = new Map<string, ToolCatalog>();

	catalogs.set('todoist', {
		fetched_at: fetchedAt,
		tools: [
			makeTool(
				'add-tasks',
				'Add one or more tasks to a Todoist project.',
				{
					content: { type: 'string', description: 'Task content.' },
					project_id: { type: 'string', description: 'Project to add the task to.' },
					due_string: { type: 'string', description: 'Natural language due date.' }
				},
				['content'],
				{ readOnlyHint: false }
			),
			makeTool(
				'complete-tasks',
				'Mark tasks as completed.',
				{
					task_id: { type: 'string', description: 'Task to complete.' }
				},
				['task_id'],
				{ readOnlyHint: false }
			),
			makeTool(
				'find-tasks',
				'Search tasks by text, project and completion state.',
				{
					query: { type: 'string', description: 'Text to search for.' },
					project_id: { type: 'string', description: 'Only search this project.' },
					limit: { type: 'integer', description: 'Maximum number of tasks.' }
				},
				[],
				{ readOnlyHint: true }
			)
		]
	});

	catalogs.set('meta', {
		fetched_at: fetchedAt,
		tools: [
			makeTool(
				'create_campaign',
				'Create a new ad campaign for a Meta ad account.',
				{
					name: { type: 'string', description: 'Campaign name.' },
					objective: { type: 'string', description: 'Campaign objective.' },
					budget: { type: 'integer', description: 'Daily budget in cents.' }
				},
				['name', 'objective'],
				{ readOnlyHint: false, destructiveHint: true }
			),
			makeTool(
				'get_ad_accounts',
				'List the Meta ad accounts available to this login.',
				{ limit: { type: 'integer', description: 'Maximum number of accounts.' } },
				[],
				{ readOnlyHint: true }
			),
			makeTool(
				'get_ad_preview',
				'Render a preview image for an ad account.',
				{ account_id: { type: 'string', description: 'Ad account id.' } },
				['account_id'],
				{ readOnlyHint: true }
			),
			makeTool(
				'get_insights',
				'Read campaign performance insights.',
				{
					account_id: { type: 'string', description: 'Ad account id.' },
					date_preset: { type: 'string', description: 'Meta date preset, like last_7d.' }
				},
				['account_id'],
				{ readOnlyHint: true }
			)
		]
	});

	catalogs.set('instagram', {
		fetched_at: fetchedAt,
		tools: [
			makeTool(
				'get_comments',
				'List recent comments on the account media.',
				{ media_id: { type: 'string', description: 'Instagram media id.' } },
				[],
				{ readOnlyHint: true }
			),
			makeTool(
				'reply_to_comment',
				'Reply to a comment on the account media.',
				{
					comment_id: { type: 'string', description: 'Comment to reply to.' },
					message: { type: 'string', description: 'Reply text.' }
				},
				['comment_id', 'message'],
				{ readOnlyHint: false }
			)
		]
	});

	catalogs.set('github', {
		fetched_at: fetchedAt,
		tools: [
			makeTool(
				'create_issue',
				'Open a new issue in a GitHub repository.',
				{
					title: { type: 'string', description: 'Issue title.' },
					body: { type: 'string', description: 'Issue body in Markdown.' }
				},
				['title'],
				{ readOnlyHint: false }
			),
			makeTool(
				'list_issues',
				'List issues from a repository.',
				{
					state: { type: 'string', description: 'open, closed or all.' },
					limit: { type: 'integer', description: 'Maximum number of issues.' }
				},
				[],
				{ readOnlyHint: true }
			)
		]
	});

	catalogs.set('billing-api', {
		fetched_at: fetchedAt,
		tools: [
			makeTool(
				'create_invoice',
				'Create an invoice for a customer.',
				{
					customer_id: { type: 'string', description: 'Customer identifier.' },
					amount: { type: 'number', description: 'Amount in dollars.' }
				},
				['customer_id', 'amount'],
				{ readOnlyHint: false }
			),
			makeTool(
				'list_invoices',
				'List invoices, optionally filtered by status.',
				{ status: { type: 'string', description: 'paid, open or void.' } },
				[],
				{ readOnlyHint: true }
			)
		]
	});

	const ops = services.find((service) => service.name === 'ops');
	catalogs.set('ops', {
		fetched_at: fetchedAt,
		tools: (ops?.command_tools ?? []).map(commandToolInfo)
	});

	const shop = services.find((service) => service.name === 'shop-api');
	catalogs.set('shop-api', {
		fetched_at: fetchedAt,
		tools: [
			...(shop?.command_tools ?? []).map(commandToolInfo),
			makeTool(
				'http_get',
				"Read-only GET request to this service's API, to explore it before adding a tool with pandora tool add --http. URLs: api (https://shop.example.com/v1)",
				{
					path: { type: 'string' },
					url: { type: 'string', enum: ['api'] },
					query: { type: 'object' }
				},
				['path'],
				{ readOnlyHint: true }
			)
		]
	});

	return catalogs;
}

function seedIntegrations(): IntegrationView[] {
	return [
		{ client: 'claude-code', detected: true, supports_skill: true, mode: 'skill' },
		{ client: 'codex', detected: true, supports_skill: true, mode: '' },
		{ client: 'opencode', detected: false, supports_skill: true, mode: '' },
		{ client: 'cursor', detected: true, supports_skill: false, mode: 'mcp' },
		{ client: 'claude-desktop', detected: false, supports_skill: false, mode: '' }
	];
}

type LogTemplate = {
	ref: string;
	tool: string;
	ok: boolean;
	blocked?: boolean;
	error?: string;
	duration: number;
};

function seedLogs(): LogEntry[] {
	const clients = ['claude-code', 'codex', 'opencode', 'mcp:cursor', 'pandora-ui'];
	const templates: LogTemplate[] = [
		{
			ref: 'todoist/personal',
			tool: 'find-tasks',
			ok: true,
			duration: 84
		},
		{
			ref: 'meta/clothing-store',
			tool: 'get_ad_accounts',
			ok: true,
			duration: 132
		},
		{
			ref: 'github/company',
			tool: 'list_issues',
			ok: true,
			duration: 247
		},
		{
			ref: 'meta/clothing-store',
			tool: 'create_campaign',
			ok: false,
			blocked: true,
			error: 'tool create_campaign is blocked because meta/clothing-store is read-only',
			duration: 4
		},
		{
			ref: 'billing-api/prod',
			tool: 'create_invoice',
			ok: true,
			duration: 1400
		},
		{
			ref: 'todoist/personal',
			tool: 'add-tasks',
			ok: false,
			error: 'todoist returned 429 Too Many Requests',
			duration: 318
		},
		{
			ref: 'ops/local',
			tool: 'disk-usage',
			ok: true,
			duration: 612
		},
		{
			ref: 'meta/shoe-store',
			tool: 'create_campaign',
			ok: false,
			blocked: true,
			error: 'tool create_campaign is not allowed by the rules for meta/shoe-store',
			duration: 6
		},
		{
			ref: 'github/company',
			tool: 'create_issue',
			ok: true,
			duration: 286
		},
		{
			ref: 'todoist/personal',
			tool: 'complete-tasks',
			ok: true,
			duration: 109
		},
		{
			ref: 'meta/shoe-store',
			tool: 'get_insights',
			ok: true,
			duration: 892
		},
		{
			ref: 'ops/local',
			tool: 'deploy',
			ok: false,
			error: 'deploy.sh exited with status 1: no such environment staging',
			duration: 2410
		}
	];

	const now = Date.now();
	const entries = Array.from({ length: 30 }, (_, index) => {
		const template = templates[index % templates.length];
		const minutes = index === 29 ? 26 * 60 : 2 + index * 23;
		const entry: LogEntry = {
			time: new Date(now - minutes * 60_000).toISOString(),
			ref: template.ref,
			tool: template.tool,
			client: clients[index % clients.length],
			duration_ms: template.duration,
			ok: template.ok
		};
		if (template.blocked) entry.blocked = true;
		if (template.error) entry.error = template.error;
		return entry;
	});

	return entries.sort((a, b) => Date.parse(a.time) - Date.parse(b.time));
}

function seedWebhooks(): WebhookInfo[] {
	if (mockMode === 'empty' || mockMode === 'webhooks-empty') return [];

	const now = Date.now();
	const lastEventAt =
		mockMode === 'events-empty' ? undefined : new Date(now - 2 * 60_000).toISOString();

	return [
		{
			ref: 'instagram/store',
			style: 'hmac',
			challenge: 'hub',
			url: 'https://mac.tail1234.ts.net/pandora/hooks/instagram/store/8f4c1c7a2d8b4e9f9a61c0d8b7e3f210',
			local_url: 'http://127.0.0.1:54645/hooks/instagram/store/8f4c1c7a2d8b4e9f9a61c0d8b7e3f210',
			last_event_at: lastEventAt
		},
		{
			ref: 'github/work',
			style: 'hmac',
			local_url: 'http://127.0.0.1:54645/hooks/github/work/4b7d5c1e9a2f4c8d8e3b7a1f6c2d9e40'
		}
	];
}

function seedEvents(): EventView[] {
	if (mockMode === 'empty' || mockMode === 'webhooks-empty' || mockMode === 'events-empty') {
		return [];
	}

	const now = Date.now();
	const at = (minutes: number): string => new Date(now - minutes * 60_000).toISOString();

	return [
		{
			seq: 12,
			id: 'evt_01jcf7k4n8q2r5t9w3x6z1a4b7',
			received_at: at(2),
			service: 'instagram',
			account: 'store',
			topic: 'comments',
			headers: {
				'Content-Type': 'application/json',
				'User-Agent': 'facebookexternalua'
			},
			payload: {
				object: 'instagram',
				entry: [
					{
						id: '17841400000000000',
						time: 1790006400,
						changes: [
							{
								field: 'comments',
								value: {
									id: '17865432109876543',
									text: 'This is exactly the reel I needed. I have been looking for a lightweight jacket like this for months, and the color is perfect. Do you ship to Lisbon, and how long does delivery usually take?',
									from: { id: '17841400000009999', username: 'trono_fan' },
									media: {
										id: '17890000000000001',
										media_product_type: 'REELS'
									},
									created_time: '2026-11-24T12:58:00+0000'
								}
							}
						]
					}
				]
			}
		},
		{
			seq: 11,
			id: 'evt_01jcf7m2p4r6s8u1v3w5x7y9z2',
			received_at: at(7),
			service: 'instagram',
			account: 'store',
			topic: 'messaging',
			headers: {
				'Content-Type': 'application/json',
				'User-Agent': 'facebookexternalua'
			},
			payload: {
				object: 'instagram',
				entry: [
					{
						id: '17841400000000000',
						time: 1790006100,
						messaging: [
							{
								sender: { id: '17841400000008888' },
								recipient: { id: '17841400000000000' },
								timestamp: 1790006100,
								message: {
									mid: 'mid.1790006100.9f4c2a',
									text: 'Do you have the jacket in a medium?'
								}
							}
						]
					}
				]
			}
		},
		{
			seq: 10,
			id: 'evt_01jcf7n8q1s3t5u7v9w1x3y5z7',
			received_at: at(14),
			service: 'instagram',
			account: 'store',
			topic: 'mentions',
			headers: {
				'Content-Type': 'application/json',
				'User-Agent': 'facebookexternalua'
			},
			payload: {
				object: 'instagram',
				entry: [
					{
						id: '17841400000000000',
						time: 1790005300,
						changes: [
							{
								field: 'mentions',
								value: {
									media_id: '17890000000000002',
									comment_id: '17865432109870001'
								}
							}
						]
					}
				]
			}
		},
		{
			seq: 9,
			id: 'evt_01jcf7p2r4t6u8v1w3x5y7z9a2',
			received_at: at(26),
			service: 'instagram',
			account: 'store',
			topic: 'comments',
			headers: {
				'Content-Type': 'application/json',
				'User-Agent': 'facebookexternalua'
			},
			payload:
				'New comment from @trono_fan: “Where can I buy this?” The comment is awaiting a reply.'
		},
		{
			seq: 8,
			id: 'evt_01jcf7q6s8u2v4w6x8y1z3a5b7',
			received_at: at(41),
			service: 'instagram',
			account: 'store',
			topic: 'messaging',
			headers: {
				'Content-Type': 'application/json',
				'User-Agent': 'facebookexternalua'
			},
			payload: {
				object: 'instagram',
				entry: [
					{
						id: '17841400000000000',
						messaging: [
							{
								sender: { id: '17841400000007777' },
								message: {
									mid: 'mid.1790004600.2a8d4f',
									text: 'Can I get the link for the black version?'
								}
							}
						]
					}
				]
			}
		},
		{
			seq: 7,
			id: 'evt_01jcf7r1t3v5w7x9y2z4a6b8c1',
			received_at: at(58),
			service: 'instagram',
			account: 'store',
			topic: 'comments',
			payload: {
				object: 'instagram',
				entry: [
					{
						changes: [
							{
								field: 'comments',
								value: { text: 'Beautiful drop!' }
							}
						]
					}
				]
			}
		}
	];
}

function doctorChecks(offline: boolean): DoctorCheck[] {
	const base: DoctorCheck[] = [
		{ name: 'Config file', ok: true, info: '~/.config/pandora/config.toml' },
		{ name: 'Secret store', ok: true, info: 'macOS Keychain is available' },
		{
			name: 'pandora on PATH',
			ok: false,
			info: 'GUI apps cannot find the pandora command.',
			fix: 'export PATH="$HOME/.local/bin:$PATH"'
		}
	];

	if (offline) return base;

	const webhookChecks: DoctorCheck[] =
		mockMode === 'empty' || mockMode === 'webhooks-empty'
			? []
			: [
					{
						name: 'tunnel',
						ok: true,
						info: 'https://mac.tail1234.ts.net/pandora'
					},
					{
						name: 'webhook receiver',
						ok: true,
						info: '127.0.0.1:54645'
					}
				];

	return [
		...base,
		...webhookChecks,
		{ name: 'todoist/personal', ok: true, info: 'Connected · 3 tools' },
		{ name: 'meta/clothing-store', ok: true, info: 'Connected · 4 tools' },
		{ name: 'ops/local', ok: true, info: 'Connected · 2 tools' }
	];
}

const mockMode =
	typeof window === 'undefined' ? null : new URLSearchParams(window.location.search).get('mock');

const services: ServiceView[] = mockMode === 'empty' ? [] : seedServices();
const webhooks: WebhookInfo[] = seedWebhooks();
const events: EventView[] = seedEvents();
let autostart: AutostartInfo = {
	supported: mockMode !== 'autostart-unsupported',
	enabled: false
};
const statuses = new Map<string, StatusInfo>();
const logins = new Map<string, MockLogin>();
let loginSequence = 0;

function setStatus(ref: string, status: Omit<StatusInfo, 'ref'>): void {
	statuses.set(ref, { ref, ...status });
}

if (mockMode !== 'empty') {
	setStatus('todoist/personal', {
		ok: true,
		needs_login: false,
		tools: 3,
		latency_ms: 84
	});
	setStatus('todoist/work', {
		ok: false,
		needs_login: true,
		tools: 0,
		latency_ms: 118
	});
	setStatus('meta/clothing-store', {
		ok: true,
		needs_login: false,
		tools: 4,
		latency_ms: 132
	});
	setStatus('meta/shoe-store', {
		ok: true,
		needs_login: false,
		tools: 4,
		latency_ms: 141
	});
	setStatus('instagram/store', {
		ok: true,
		needs_login: false,
		tools: 2,
		latency_ms: 166
	});
	setStatus('github/company', {
		ok: false,
		needs_login: false,
		tools: 0,
		latency_ms: 97,
		error: 'spawn npx: connect: connection refused'
	});
	setStatus('github/work', {
		ok: true,
		needs_login: false,
		tools: 2,
		latency_ms: 104
	});
	setStatus('billing-api/prod', {
		ok: true,
		needs_login: false,
		tools: 2,
		latency_ms: 188
	});
	setStatus('ops/local', {
		ok: true,
		needs_login: false,
		tools: 2,
		latency_ms: 41
	});
}

const catalogs = mockMode === 'empty' ? new Map<string, ToolCatalog>() : seedCatalogs();
const logs: LogEntry[] = mockMode === 'empty' ? [] : seedLogs();
const integrations: IntegrationView[] = seedIntegrations();

function findService(name: string): ServiceView | undefined {
	return services.find((service) => service.name === name);
}

function findAccount(service: ServiceView, name: string): AccountView | undefined {
	return service.accounts.find((account) => account.name === name);
}

function defaultAccountName(service: ServiceView): string | undefined {
	return service.accounts.find((account) => account.default)?.name ?? service.accounts[0]?.name;
}

function validateName(name: string, label: string): void {
	if (!NAME_PATTERN.test(name)) {
		throw new ApiError(
			`${label} must use lowercase letters, digits and dashes, up to 32 characters.`,
			400
		);
	}
}

function serviceAuth(input: AddAccountInput, kind: ServiceKind): AuthMode {
	if (kind === 'mcp-http' || kind === 'openapi') {
		if (input.auth === 'oauth') return 'oauth';
		if (input.secret_headers && Object.keys(input.secret_headers).length > 0) return 'header';
	}
	return 'none';
}

function normalizedUrls(urls: Record<string, string> | undefined): Record<string, string> {
	const entries = Object.entries(urls ?? {});
	if (entries.length === 0) {
		throw new ApiError('An HTTP service needs at least one URL.', 400);
	}
	const result: Record<string, string> = {};
	for (const [name, raw] of entries) {
		validateName(name, 'URL name');
		result[name] = checkedBaseUrl(raw);
	}
	return result;
}

function checkedBaseUrl(raw: string): string {
	let parsed: URL;
	try {
		parsed = new URL(raw.trim());
	} catch {
		throw new ApiError('invalid URL: check the scheme and host', 400);
	}
	const local = ['localhost', '127.0.0.1', '[::1]'].includes(parsed.hostname);
	if (parsed.protocol !== 'https:' && !(parsed.protocol === 'http:' && local)) {
		throw new ApiError('the URL must use https (http is allowed only for localhost)', 400);
	}
	if (parsed.search || parsed.hash) {
		throw new ApiError('the URL cannot contain a query or a fragment', 400);
	}
	return raw.trim().replace(/\/+$/, '');
}

function httpServiceOrThrow(serviceName: string): ServiceView {
	const service = findService(serviceName);
	if (!service) throw new ApiError(`unknown service "${serviceName}"`, 404);
	if (service.kind !== 'http') {
		throw new ApiError(
			`service ${serviceName} is not an HTTP service: create one with pandora add ${serviceName} --as <account> --http name=URL`,
			400
		);
	}
	return service;
}

async function handleSetServiceUrl(
	serviceName: string,
	name: string,
	body: unknown
): Promise<void> {
	await delay(300);
	const service = httpServiceOrThrow(serviceName);
	validateName(name, 'URL name');
	const raw =
		typeof (body as { url?: unknown })?.url === 'string' ? (body as { url: string }).url : '';
	service.urls = { ...(service.urls ?? {}), [name]: checkedBaseUrl(raw) };
	refreshCommandCatalog(service);
}

async function handleRemoveServiceUrl(serviceName: string, name: string): Promise<void> {
	await delay(300);
	const service = httpServiceOrThrow(serviceName);
	const urls = { ...(service.urls ?? {}) };
	if (!(name in urls))
		throw new ApiError(`URL ${name} does not exist in service ${serviceName}`, 404);
	if (Object.keys(urls).length === 1) {
		throw new ApiError(
			`an HTTP service needs at least one URL; remove the service with pandora remove ${serviceName} --yes instead`,
			400
		);
	}
	const user = (service.command_tools ?? []).find((tool) => tool.http?.url === name);
	if (user) throw new ApiError(`tool ${user.name} uses URL ${name}: remove the tool first`, 400);
	delete urls[name];
	service.urls = urls;
	refreshCommandCatalog(service);
}

function canonicalHeader(name: string): string {
	return name
		.trim()
		.split('-')
		.map((part) => part.charAt(0).toUpperCase() + part.slice(1).toLowerCase())
		.join('-');
}

async function handleSetServiceHeader(
	serviceName: string,
	name: string,
	body: unknown
): Promise<void> {
	await delay(300);
	const service = httpServiceOrThrow(serviceName);
	const template =
		typeof (body as { template?: unknown })?.template === 'string'
			? (body as { template: string }).template
			: '';
	if (!/^[!#$%&'*+.^_`|~0-9A-Za-z-]+$/.test(name)) {
		throw new ApiError(`invalid header name "${name}": use a valid HTTP token`, 400);
	}
	if (/\{\{(?!env\.)/.test(template)) {
		throw new ApiError('service headers can only use {{env.NAME}}', 400);
	}
	service.headers = { ...(service.headers ?? {}), [canonicalHeader(name)]: template };
}

async function handleRemoveServiceHeader(serviceName: string, name: string): Promise<void> {
	await delay(300);
	const service = httpServiceOrThrow(serviceName);
	const key = canonicalHeader(name);
	const headers = { ...(service.headers ?? {}) };
	if (!(key in headers))
		throw new ApiError(`header ${key} does not exist in service ${serviceName}`, 404);
	delete headers[key];
	service.headers = headers;
}

function accountOrThrow(serviceName: string, accountName: string): AccountView {
	const service = findService(serviceName);
	const account = service ? findAccount(service, accountName) : undefined;
	if (!account) throw new ApiError(`unknown account "${serviceName}/${accountName}"`, 404);
	return account;
}

async function handleSetAccountEnv(
	serviceName: string,
	accountName: string,
	name: string,
	body: unknown
): Promise<void> {
	await delay(300);
	const account = accountOrThrow(serviceName, accountName);
	if (!/^[A-Za-z_][A-Za-z0-9_]*$/.test(name)) {
		throw new ApiError(
			`invalid environment name "${name}": use letters, digits and underscores, starting with a letter or underscore`,
			400
		);
	}
	if (account.secret_env?.includes(name)) {
		throw new ApiError(
			`${name} is a secret of ${serviceName}/${accountName}: change it with pandora remove and add`,
			400
		);
	}
	const value =
		typeof (body as { value?: unknown })?.value === 'string'
			? (body as { value: string }).value
			: '';
	account.env = { ...(account.env ?? {}), [name]: value };
}

async function handleUnsetAccountEnv(
	serviceName: string,
	accountName: string,
	name: string
): Promise<void> {
	await delay(300);
	const account = accountOrThrow(serviceName, accountName);
	const env = { ...(account.env ?? {}) };
	if (!(name in env))
		throw new ApiError(`${name} is not set for ${serviceName}/${accountName}`, 404);
	delete env[name];
	account.env = env;
}

function initialTools(kind: ServiceKind): ToolInfo[] {
	if (kind === 'command' || kind === 'http') return [];
	if (kind === 'openapi') {
		return [makeTool('get_items', 'List items from the API.', {}, [], { readOnlyHint: true })];
	}
	if (kind === 'mcp-stdio') {
		return [
			makeTool('status', 'Check that the local MCP server is running.', {}, [], {
				readOnlyHint: true
			})
		];
	}
	return [
		makeTool('ping', 'Check that the MCP server is reachable.', {}, [], {
			readOnlyHint: true
		})
	];
}

function statusForAccount(service: ServiceView, account: AccountView): StatusInfo {
	const ref = refOf(service.name, account.name);

	if (service.auth === 'oauth') {
		return { ref, ok: false, needs_login: true, tools: 0, latency_ms: 160 };
	}

	if (service.name === 'github' && service.kind === 'mcp-stdio') {
		return {
			ref,
			ok: false,
			needs_login: false,
			tools: 0,
			latency_ms: 122,
			error: 'spawn npx: connect: connection refused'
		};
	}

	return {
		ref,
		ok: true,
		needs_login: false,
		tools: service.tool_count,
		latency_ms: 92
	};
}

function renameStatusRef(oldRef: string, newRef: string): void {
	const status = statuses.get(oldRef);
	if (!status) return;

	statuses.delete(oldRef);
	statuses.set(newRef, { ...status, ref: newRef });
}

function overviewSnapshot(): Overview {
	return structuredClone({
		version: MOCK_VERSION,
		services,
		integrations: MOCK_INTEGRATIONS
	});
}

async function handleAddAccount(body: unknown): Promise<unknown> {
	const input = (body ?? {}) as AddAccountInput;
	const serviceName = typeof input.service === 'string' ? input.service.trim() : '';
	const accountName = typeof input.account === 'string' ? input.account.trim() : '';

	validateName(serviceName, 'Service name');
	validateName(accountName, 'Account name');

	await delay(500);

	let service = findService(serviceName);

	if (input.kind) {
		if (service) {
			throw new ApiError(`Service ${serviceName} already exists.`, 400);
		}

		const kind = input.kind;
		const tools = initialTools(kind);
		service = {
			name: serviceName,
			kind,
			description: input.service_description?.trim() || undefined,
			auth: serviceAuth(input, kind),
			expose: 'compact',
			url: input.url?.trim() || undefined,
			command: kind === 'mcp-stdio' ? input.command : undefined,
			spec: kind === 'openapi' ? input.spec?.trim() || undefined : undefined,
			urls: kind === 'http' ? normalizedUrls(input.urls) : undefined,
			headers: kind === 'http' ? {} : undefined,
			tools_known: kind !== 'command',
			tool_count: tools.length,
			command_tools: kind === 'command' || kind === 'http' ? [] : undefined,
			accounts: []
		};
		services.push(service);
		catalogs.set(serviceName, {
			fetched_at: new Date().toISOString(),
			tools
		});
		if (kind === 'http') refreshCommandCatalog(service);
	} else if (!service) {
		throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	}

	if (findAccount(service, accountName)) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} already exists.`, 400);
	}

	const isFirstAccount = service.accounts.length === 0;
	const account: AccountView = {
		name: accountName,
		description: input.description?.trim() || undefined,
		default: isFirstAccount,
		readonly: false,
		allow: [],
		deny: [],
		env: { ...(input.env ?? {}) },
		secret_env: Object.keys(input.secret_env ?? {}).sort(),
		secret_headers: Object.keys(input.secret_headers ?? {}).sort()
	};

	service.accounts.push(account);
	if (account.default) service.default_account = account.name;

	const status = statusForAccount(service, account);
	setStatus(status.ref, status);

	return {
		ref: status.ref,
		status,
		warning:
			service.kind === 'command' || service.kind === 'http'
				? 'Tools for this service are added after it is created.'
				: undefined
	};
}

async function handleDeleteAccount(serviceName: string, accountName: string): Promise<void> {
	await delay(350);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	const index = service.accounts.findIndex((account) => account.name === accountName);
	if (index === -1) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}

	service.accounts.splice(index, 1);
	statuses.delete(refOf(serviceName, accountName));

	if (service.accounts.length > 0 && !service.accounts.some((account) => account.default)) {
		service.accounts[0].default = true;
	}
	service.default_account = defaultAccountName(service);
}

async function handleDeleteService(serviceName: string): Promise<void> {
	await delay(350);
	const index = services.findIndex((service) => service.name === serviceName);
	if (index === -1) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	services.splice(index, 1);
	catalogs.delete(serviceName);
	for (const ref of [...statuses.keys()]) {
		if (ref.startsWith(`${serviceName}/`)) statuses.delete(ref);
	}
}

async function handleRenameAccount(
	serviceName: string,
	accountName: string,
	body: unknown
): Promise<void> {
	const record = (body ?? {}) as { name?: string };
	const name = typeof record.name === 'string' ? record.name.trim() : '';
	validateName(name, 'Account name');

	await delay(350);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	const account = findAccount(service, accountName);
	if (!account) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}
	if (findAccount(service, name)) {
		throw new ApiError(`Account ${refOf(serviceName, name)} already exists.`, 400);
	}

	const wasDefault = account.default;
	account.name = name;
	if (wasDefault) service.default_account = name;

	renameStatusRef(refOf(serviceName, accountName), refOf(serviceName, name));
}

async function handleRenameService(serviceName: string, body: unknown): Promise<void> {
	const record = (body ?? {}) as { name?: string };
	const name = typeof record.name === 'string' ? record.name.trim() : '';
	validateName(name, 'Service name');

	await delay(350);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	if (findService(name)) throw new ApiError(`Service ${name} already exists.`, 400);

	service.name = name;

	const catalog = catalogs.get(serviceName);
	if (catalog) {
		catalogs.delete(serviceName);
		catalogs.set(name, catalog);
	}

	for (const ref of [...statuses.keys()]) {
		if (!ref.startsWith(`${serviceName}/`)) continue;
		const accountName = ref.slice(serviceName.length + 1);
		renameStatusRef(ref, refOf(name, accountName));
	}
}

async function handleSetDefault(serviceName: string, accountName: string): Promise<void> {
	await delay(300);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	const account = findAccount(service, accountName);
	if (!account) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}

	for (const item of service.accounts) item.default = item.name === accountName;
	service.default_account = accountName;
}

async function handleDescription(body: unknown): Promise<void> {
	const record = (body ?? {}) as { ref?: string; text?: string };
	const ref = typeof record.ref === 'string' ? record.ref : '';
	const text = typeof record.text === 'string' ? record.text.trim() : '';
	const separator = ref.indexOf('/');

	await delay(300);

	if (separator === -1) {
		const service = findService(ref);
		if (!service) throw new ApiError(`Service ${ref} does not exist.`, 404);
		service.description = text || undefined;
		return;
	}

	const serviceName = ref.slice(0, separator);
	const accountName = ref.slice(separator + 1);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	const account = findAccount(service, accountName);
	if (!account) throw new ApiError(`Account ${ref} does not exist.`, 404);
	account.description = text || undefined;
}

async function handleStartLogin(serviceName: string, accountName: string): Promise<unknown> {
	await delay(250);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	if (!findAccount(service, accountName)) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}

	const loginId = `login-${++loginSequence}`;
	logins.set(loginId, {
		ref: refOf(serviceName, accountName),
		startedAt: Date.now()
	});

	return { login_id: loginId };
}

async function handleLoginStatus(loginId: string): Promise<LoginResult> {
	await delay(200);
	const login = logins.get(loginId);
	if (!login) throw new ApiError('Login not found.', 404);

	if (Date.now() - login.startedAt < 3000) return { state: 'pending' };

	const separator = login.ref.indexOf('/');
	const service = findService(login.ref.slice(0, separator));
	const accountName = login.ref.slice(separator + 1);
	const account = service ? findAccount(service, accountName) : undefined;

	if (!service || !account) {
		logins.delete(loginId);
		return { state: 'error', error: 'The account was removed before the login finished.' };
	}

	setStatus(login.ref, {
		ok: true,
		needs_login: false,
		tools: service.tool_count,
		latency_ms: 176
	});
	logins.delete(loginId);
	return { state: 'done' };
}

async function handleLogout(serviceName: string, accountName: string): Promise<void> {
	await delay(300);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	if (!findAccount(service, accountName)) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}

	setStatus(refOf(serviceName, accountName), {
		ok: false,
		needs_login: true,
		tools: 0,
		latency_ms: 0
	});
}

function requireAccount(
	serviceName: string,
	accountName: string
): { service: ServiceView; account: AccountView } {
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	const account = findAccount(service, accountName);
	if (!account) {
		throw new ApiError(`Account ${refOf(serviceName, accountName)} does not exist.`, 404);
	}

	return { service, account };
}

function matchesRule(pattern: string, tool: string): boolean {
	const escaped = pattern.replace(/[.+^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*');
	return new RegExp(`^${escaped}$`).test(tool);
}

function ruleViolation(account: AccountView, tool: ToolInfo, ref: string): string | null {
	if (account.readonly && tool.annotations?.readOnlyHint !== true) {
		return `tool ${tool.name} is blocked because ${ref} is read-only`;
	}

	const denied = account.deny.find((pattern) => matchesRule(pattern, tool.name));
	if (denied) {
		return `tool ${tool.name} is blocked by the deny rule ${denied} for ${ref}`;
	}

	if (
		account.allow.length > 0 &&
		!account.allow.some((pattern) => matchesRule(pattern, tool.name))
	) {
		return `tool ${tool.name} is not allowed by the rules for ${ref}`;
	}

	return null;
}

function cleanPatterns(value: unknown): string[] {
	if (!Array.isArray(value)) return [];
	const unique = new Set(
		value
			.filter((entry): entry is string => typeof entry === 'string')
			.map((entry) => entry.trim())
			.filter(Boolean)
	);
	return [...unique];
}

function addLog(entry: LogEntry): void {
	logs.push(entry);
}

function argText(value: JsonValue | undefined, fallback: string): string {
	if (value === undefined || value === null || value === '') return fallback;
	return String(value);
}

function previewContent(): ToolContent {
	const svg =
		'<svg xmlns="http://www.w3.org/2000/svg" width="320" height="180" viewBox="0 0 320 180"><rect width="320" height="180" rx="12" fill="#f4f4f5"/><rect x="20" y="20" width="120" height="24" rx="6" fill="#7c3aed"/><rect x="20" y="60" width="280" height="12" rx="6" fill="#d4d4d8"/><rect x="20" y="82" width="220" height="12" rx="6" fill="#d4d4d8"/><rect x="20" y="120" width="90" height="36" rx="8" fill="#a1a1aa"/><text x="160" y="105" text-anchor="middle" font-family="sans-serif" font-size="14" fill="#52525b">Ad preview</text></svg>';
	return { type: 'image', data: btoa(svg), mimeType: 'image/svg+xml' };
}

function toolContent(tool: ToolInfo, args: Record<string, JsonValue>): ToolContent[] {
	switch (tool.name) {
		case 'find-tasks':
			return [
				{
					type: 'text',
					text: JSON.stringify(
						{
							query: argText(args.query, 'all tasks'),
							tasks: [
								{ id: '8842', content: 'Buy coffee', project: 'errands' },
								{ id: '8839', content: 'Review release notes', project: 'work' },
								{ id: '8831', content: 'Book dentist', project: 'health' }
							]
						},
						null,
						2
					)
				}
			];
		case 'add-tasks':
			return [
				{
					type: 'text',
					text: `Added task: ${argText(args.content, 'Untitled task')}`
				}
			];
		case 'complete-tasks':
			return [
				{
					type: 'text',
					text: `Completed task ${argText(args.task_id, 'unknown')}.`
				}
			];
		case 'get_ad_accounts':
			return [
				{
					type: 'text',
					text: JSON.stringify(
						[
							{ id: 'act_12001', name: 'Clothing Store', currency: 'BRL' },
							{ id: 'act_12002', name: 'Shoe Store', currency: 'BRL' }
						],
						null,
						2
					)
				}
			];
		case 'get_insights':
			return [
				{
					type: 'text',
					text: JSON.stringify(
						{
							spend: 184.52,
							impressions: 12034,
							clicks: 421,
							ctr: 0.035
						},
						null,
						2
					)
				}
			];
		case 'get_ad_preview':
			return [{ type: 'text', text: 'Preview generated for account act_12001.' }, previewContent()];
		case 'create_campaign':
			return [
				{
					type: 'text',
					text: `Created campaign "${argText(args.name, 'New campaign')}".`
				}
			];
		case 'list_issues':
			return [
				{
					type: 'text',
					text: JSON.stringify(
						[
							{ number: 482, title: 'Flaky sync test', state: 'open' },
							{ number: 479, title: 'Document MCP setup', state: 'open' }
						],
						null,
						2
					)
				}
			];
		case 'create_issue':
			return [
				{
					type: 'text',
					text: `Opened issue #483: ${argText(args.title, 'Untitled issue')}`
				}
			];
		case 'list_invoices':
			return [
				{
					type: 'text',
					text: JSON.stringify(
						[
							{ id: 'INV-1042', customer: 'Acme', amount: 420, status: 'paid' },
							{ id: 'INV-1043', customer: 'Globex', amount: 199, status: 'open' }
						],
						null,
						2
					)
				}
			];
		case 'create_invoice':
			return [
				{
					type: 'text',
					text: `Created invoice INV-1044 for ${argText(args.customer_id, 'customer')}.`
				}
			];
		case 'disk-usage':
			return [
				{
					type: 'text',
					text: 'Filesystem      Size  Used Avail Capacity\n/dev/disk3s1   460Gi 312Gi 118Gi    73%'
				}
			];
		case 'deploy':
			return [
				{
					type: 'text',
					text: `Deployed to ${argText(args.environment, 'production')}.`
				}
			];
		case 'list-orders':
			return [
				{
					type: 'text',
					text: JSON.stringify({
						data: [
							{ id: 'ord_1042', status: 'paid', total: 129.9, customer: { name: 'Ana Souza' } },
							{ id: 'ord_1041', status: 'shipped', total: 54.5, customer: { name: 'Bruno Lima' } }
						],
						has_more: false
					})
				}
			];
		default:
			return [{ type: 'text', text: `Called ${tool.name}.` }];
	}
}

async function handleTools(serviceName: string, refresh: boolean): Promise<ToolCatalog> {
	await delay(500);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);

	let catalog = catalogs.get(serviceName);
	if (!catalog) {
		catalog = { fetched_at: new Date().toISOString(), tools: [] };
		catalogs.set(serviceName, catalog);
	}

	if (refresh) catalog.fetched_at = new Date().toISOString();
	return structuredClone(catalog);
}

async function handleCall(body: unknown): Promise<CallToolResult> {
	const input = (body ?? {}) as CallToolInput;
	const ref = typeof input.ref === 'string' ? input.ref : '';
	const toolName = typeof input.tool === 'string' ? input.tool : '';
	const separator = ref.indexOf('/');

	if (separator === -1 || !toolName) {
		throw new ApiError('ref and tool are required', 400);
	}

	const serviceName = ref.slice(0, separator);
	const accountName = ref.slice(separator + 1);
	const { account } = requireAccount(serviceName, accountName);
	const catalog = catalogs.get(serviceName);
	const tool = catalog?.tools.find((entry) => entry.name === toolName);
	if (!tool) throw new ApiError(`Tool ${toolName} does not exist.`, 404);

	const started = Date.now();
	await delay(450);
	const reason = ruleViolation(account, tool, ref);

	if (reason) {
		addLog({
			time: new Date().toISOString(),
			ref,
			tool: toolName,
			client: 'pandora-ui',
			duration_ms: Math.max(1, Date.now() - started),
			ok: false,
			blocked: true,
			error: reason
		});
		throw new ApiError(reason, 403);
	}

	const args = (input.args ?? {}) as Record<string, JsonValue>;
	addLog({
		time: new Date().toISOString(),
		ref,
		tool: toolName,
		client: 'pandora-ui',
		duration_ms: Math.max(1, Date.now() - started),
		ok: true
	});

	return { content: toolContent(tool, args), isError: false };
}

async function handleGetPolicy(serviceName: string, accountName: string): Promise<AccountPolicy> {
	await delay(300);
	const { account } = requireAccount(serviceName, accountName);
	return {
		readonly: account.readonly,
		allow: [...account.allow],
		deny: [...account.deny]
	};
}

async function handlePutPolicy(
	serviceName: string,
	accountName: string,
	body: unknown
): Promise<AccountPolicy> {
	const input = (body ?? {}) as Partial<AccountPolicy>;
	await delay(400);
	const { account } = requireAccount(serviceName, accountName);
	account.readonly = Boolean(input.readonly);
	account.allow = cleanPatterns(input.allow);
	account.deny = cleanPatterns(input.deny);
	return {
		readonly: account.readonly,
		allow: [...account.allow],
		deny: [...account.deny]
	};
}

async function handleLog(url: URL): Promise<LogEntry[]> {
	await delay(300);
	const rawLimit = Number(url.searchParams.get('n') ?? '200');
	const limit = Number.isFinite(rawLimit) && rawLimit >= 0 ? rawLimit : 200;
	const ref = url.searchParams.get('ref') ?? '';
	const onlyErrors = url.searchParams.get('errors') === '1';

	const filtered = logs.filter((entry) => {
		if (onlyErrors && entry.ok) return false;
		if (!ref) return true;
		return ref.includes('/') ? entry.ref === ref : entry.ref.startsWith(`${ref}/`);
	});

	return structuredClone(limit === 0 ? [] : filtered.slice(-limit));
}

async function handleIntegrations(): Promise<IntegrationView[]> {
	await delay(400);
	return structuredClone(integrations);
}

function installReport(integration: IntegrationView, mode: 'skill' | 'mcp'): InstallReport {
	const report: InstallReport = { client: integration.client, mode };

	if (mode === 'skill') {
		report.skill_dir = `~/.config/pandora/skills/${integration.client}`;
		if (integration.client === 'codex') {
			report.permission_note =
				'Codex may ask you to approve the pandora command the first time it runs.';
		}
	} else {
		report.mcp_config = `~/.config/pandora/mcp/${integration.client}.json`;
	}

	return report;
}

async function handleInstall(clientName: string, body: unknown): Promise<InstallReport[]> {
	const integration = integrations.find((entry) => entry.client === clientName);
	if (!integration) throw new ApiError(`Unknown client ${clientName}.`, 404);

	const input = (body ?? {}) as { mcp?: boolean };
	const mode = input.mcp || !integration.supports_skill ? 'mcp' : 'skill';
	await delay(600);
	integration.mode = mode;
	return [installReport(integration, mode)];
}

async function handleUninstall(clientName: string): Promise<InstallReport[]> {
	const integration = integrations.find((entry) => entry.client === clientName);
	if (!integration) throw new ApiError(`Unknown client ${clientName}.`, 404);

	await delay(400);
	integration.mode = '';
	return [{ client: integration.client, mode: '' }];
}

async function handleDoctor(offline: boolean): Promise<DoctorCheck[]> {
	await delay(offline ? 300 : 1100);
	return doctorChecks(offline);
}

function httpGetTool(service: ServiceView): ToolInfo {
	const names = Object.keys(service.urls ?? {}).sort();
	const list = names.map((name) => `${name} (${service.urls?.[name]})`).join(', ');
	return makeTool(
		'http_get',
		`Read-only GET request to this service's API, to explore it before adding a tool with pandora tool add --http. URLs: ${list}`,
		{
			path: { type: 'string', description: 'Path after the base URL, starting with /' },
			url: { type: 'string', enum: names, description: 'Name of the base URL to call' },
			query: { type: 'object', description: 'Query parameters as name to value' }
		},
		names.length > 1 ? ['path', 'url'] : ['path'],
		{ readOnlyHint: true }
	);
}

function refreshCommandCatalog(service: ServiceView): void {
	const tools = (service.command_tools ?? []).map(commandToolInfo);
	if (service.kind === 'http' && Object.keys(service.urls ?? {}).length > 0) {
		tools.push(httpGetTool(service));
	}
	tools.sort((a, b) => a.name.localeCompare(b.name));
	const existing = catalogs.get(service.name);
	const catalog: ToolCatalog = {
		fetched_at: new Date().toISOString(),
		tools
	};
	if (existing?.instructions) catalog.instructions = existing.instructions;
	catalogs.set(service.name, catalog);
	service.tool_count = tools.length;
	service.tools_known = true;

	for (const account of service.accounts) {
		const ref = refOf(service.name, account.name);
		const status = statuses.get(ref);
		if (status) statuses.set(ref, { ...status, tools: tools.length });
	}
}

async function handleAddCommandTool(serviceName: string, body: unknown): Promise<void> {
	const input = (body ?? {}) as CommandToolInput;
	const name = typeof input.name === 'string' ? input.name.trim() : '';
	const command = Array.isArray(input.command) ? input.command : [];
	const http = input.http;

	if (!name) throw new ApiError('Tool name is required.', 400);
	if (command.length > 0 && http) {
		throw new ApiError(`tool "${name}": use a command or an HTTP request, not both`, 400);
	}
	if (command.length === 0 && !http) {
		throw new ApiError(`tool "${name}" needs a command or an HTTP request`, 400);
	}

	await delay(400);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	if (http && service.kind !== 'http') {
		throw new ApiError(
			`service ${serviceName} has no URL: add one with pandora http url ${serviceName} <name> <url>`,
			400
		);
	}
	if (!http && service.kind !== 'command' && service.kind !== 'http') {
		throw new ApiError(`service "${serviceName}" is not a command or HTTP service`, 400);
	}
	if (http) {
		const urlNames = Object.keys(service.urls ?? {});
		if (!http.url && urlNames.length === 1) http.url = urlNames[0];
		if (!http.url) throw new ApiError('the service has several URLs: choose one with --url', 400);
		if (!urlNames.includes(http.url)) {
			throw new ApiError(`unknown URL "${http.url}": add it with pandora http url`, 400);
		}
		if (!http.path?.startsWith('/'))
			throw new ApiError(`tool ${name}: path must start with /`, 400);
		http.method = http.method.toUpperCase() as typeof http.method;
		if ((http.method === 'GET' || http.method === 'HEAD') && (http.form || http.json)) {
			throw new ApiError(`tool ${name}: ${http.method} cannot have a form or JSON body`, 400);
		}
		for (const secret of service.accounts.flatMap((account) => account.secret_env ?? [])) {
			const used = [
				http.path,
				...Object.values(http.query ?? {}),
				...Object.values(http.form ?? {}),
				...Object.values(http.json ?? {}),
				...Object.values(http.headers ?? {})
			].some((value) => value.includes(`{{env.${secret}}}`));
			if (used) {
				throw new ApiError(
					`tool ${name}: the secret ${secret} can only be used in the service headers`,
					400
				);
			}
		}
	}

	service.command_tools = service.command_tools ?? [];
	if (service.command_tools.some((tool) => tool.name === name)) {
		throw new ApiError(`Tool ${name} already exists.`, 400);
	}

	const timeout = input.timeout_sec && input.timeout_sec > 0 ? input.timeout_sec : 60;
	const tool: CommandToolView = {
		name,
		description: input.description?.trim() || undefined,
		command: http ? undefined : command,
		http,
		params: input.params,
		readonly: Boolean(input.readonly) || http?.method === 'GET' || http?.method === 'HEAD',
		timeout_sec: timeout
	};
	service.command_tools.push(tool);
	refreshCommandCatalog(service);
}

async function handleRemoveCommandTool(serviceName: string, name: string): Promise<void> {
	await delay(400);
	const service = findService(serviceName);
	if (!service) throw new ApiError(`Service ${serviceName} does not exist.`, 404);
	if (service.kind !== 'command' && service.kind !== 'http') {
		throw new ApiError('Tools can only be removed from command or HTTP services.', 400);
	}

	const existing = service.command_tools ?? [];
	if (!existing.some((tool) => tool.name === name)) {
		throw new ApiError(`Tool ${name} does not exist.`, 404);
	}

	service.command_tools = existing.filter((tool) => tool.name !== name);
	refreshCommandCatalog(service);
}

export async function mockRequest(
	method: HttpMethod,
	path: string,
	body?: unknown
): Promise<unknown> {
	const url = new URL(path, 'http://mock.local');
	const segments = url.pathname.split('/').filter(Boolean).map(decodeURIComponent);

	if (method === 'GET' && url.pathname === '/api/overview') {
		await delay(400);
		if (mockMode === 'error') {
			throw new ApiError('The mock server could not load services.', 500);
		}
		return overviewSnapshot();
	}

	if (method === 'GET' && url.pathname === '/api/webhooks') {
		await delay(250);
		return structuredClone(webhooks);
	}

	if (method === 'GET' && url.pathname === '/api/events') {
		await delay(300);
		const service = url.searchParams.get('service') ?? '';
		const account = url.searchParams.get('account') ?? '';
		const topic = url.searchParams.get('topic') ?? '';
		const rawLimit = Number(url.searchParams.get('limit') ?? '100');
		const limit = Number.isFinite(rawLimit) ? Math.min(Math.max(1, rawLimit), 100) : 100;
		const filtered = events.filter((event) => {
			if (service && event.service !== service) return false;
			if (account && event.account !== account) return false;
			if (topic && event.topic !== topic) return false;
			return true;
		});
		return structuredClone(filtered.slice(0, limit));
	}

	if (method === 'GET' && url.pathname === '/api/autostart') {
		await delay(200);
		return { ...autostart };
	}

	if (method === 'PUT' && url.pathname === '/api/autostart') {
		await delay(300);
		const input = (body ?? {}) as { enabled?: unknown };
		autostart = { ...autostart, enabled: Boolean(input.enabled) };
		return { ...autostart };
	}

	if (method === 'GET' && url.pathname === '/api/status') {
		await delay(1500);
		const result: StatusInfo[] = [];

		for (const service of services) {
			for (const account of service.accounts) {
				const ref = refOf(service.name, account.name);
				const existing = statuses.get(ref);
				if (existing) {
					result.push(structuredClone(existing));
					continue;
				}

				const status: StatusInfo = {
					ref,
					ok: true,
					needs_login: false,
					tools: service.tool_count,
					latency_ms: 90
				};
				statuses.set(ref, status);
				result.push(structuredClone(status));
			}
		}

		const ref = url.searchParams.get('ref');
		return ref ? result.filter((status) => status.ref === ref) : result;
	}

	if (
		method === 'DELETE' &&
		segments[0] === 'api' &&
		segments[1] === 'accounts' &&
		segments.length === 5 &&
		segments[4] === 'webhook'
	) {
		await delay(300);
		const ref = refOf(segments[2], segments[3]);
		const index = webhooks.findIndex((webhook) => webhook.ref === ref);
		if (index === -1) throw new ApiError(`Webhook ${ref} does not exist.`, 404);
		webhooks.splice(index, 1);
		return { ok: true };
	}

	if (method === 'POST' && url.pathname === '/api/accounts') {
		return handleAddAccount(body);
	}

	if (method === 'POST' && url.pathname === '/api/call') {
		return handleCall(body);
	}

	if (method === 'GET' && url.pathname === '/api/integrations') {
		return handleIntegrations();
	}

	if (method === 'GET' && url.pathname === '/api/log') {
		return handleLog(url);
	}

	if (method === 'GET' && url.pathname === '/api/doctor') {
		return handleDoctor(url.searchParams.get('offline') === '1');
	}

	if (segments[0] === 'api' && segments[1] === 'integrations' && segments.length >= 3) {
		const clientName = segments[2];
		if (method === 'POST' && segments[3] === 'install') {
			return handleInstall(clientName, body);
		}
		if (method === 'DELETE' && segments.length === 3) {
			return handleUninstall(clientName);
		}
	}

	if (segments[0] === 'api' && segments[1] === 'accounts' && segments.length >= 4) {
		const serviceName = segments[2];
		const accountName = segments[3];

		if (segments[4] === 'policy') {
			if (method === 'GET') return handleGetPolicy(serviceName, accountName);
			if (method === 'PUT') return handlePutPolicy(serviceName, accountName, body);
		}
		if (method === 'DELETE' && segments.length === 4) {
			await handleDeleteAccount(serviceName, accountName);
			return undefined;
		}
		if (method === 'POST' && segments[4] === 'rename') {
			await handleRenameAccount(serviceName, accountName, body);
			return undefined;
		}
		if (method === 'POST' && segments[4] === 'default') {
			await handleSetDefault(serviceName, accountName);
			return undefined;
		}
		if (method === 'POST' && segments[4] === 'login') {
			return handleStartLogin(serviceName, accountName);
		}
		if (method === 'POST' && segments[4] === 'logout') {
			await handleLogout(serviceName, accountName);
			return undefined;
		}
		if (segments[4] === 'env' && segments[5]) {
			if (method === 'PUT') {
				await handleSetAccountEnv(serviceName, accountName, segments[5], body);
				return undefined;
			}
			if (method === 'DELETE') {
				await handleUnsetAccountEnv(serviceName, accountName, segments[5]);
				return undefined;
			}
		}
	}

	if (segments[0] === 'api' && segments[1] === 'services' && segments.length >= 3) {
		const serviceName = segments[2];

		if (method === 'GET' && segments[3] === 'tools' && segments.length === 4) {
			return handleTools(serviceName, url.searchParams.get('refresh') === '1');
		}
		if (method === 'POST' && segments[3] === 'command-tools' && segments.length === 4) {
			await handleAddCommandTool(serviceName, body);
			return undefined;
		}
		if (method === 'DELETE' && segments[3] === 'command-tools' && segments[4]) {
			await handleRemoveCommandTool(serviceName, segments[4]);
			return undefined;
		}
		if (segments[3] === 'urls' && segments[4]) {
			if (method === 'PUT') {
				await handleSetServiceUrl(serviceName, segments[4], body);
				return undefined;
			}
			if (method === 'DELETE') {
				await handleRemoveServiceUrl(serviceName, segments[4]);
				return undefined;
			}
		}
		if (segments[3] === 'headers' && segments[4]) {
			if (method === 'PUT') {
				await handleSetServiceHeader(serviceName, segments[4], body);
				return undefined;
			}
			if (method === 'DELETE') {
				await handleRemoveServiceHeader(serviceName, segments[4]);
				return undefined;
			}
		}
		if (method === 'DELETE' && segments.length === 3) {
			await handleDeleteService(serviceName);
			return undefined;
		}
		if (method === 'POST' && segments[3] === 'rename') {
			await handleRenameService(serviceName, body);
			return undefined;
		}
	}

	if (method === 'PUT' && url.pathname === '/api/descriptions') {
		await handleDescription(body);
		return undefined;
	}

	if (method === 'GET' && segments[0] === 'api' && segments[1] === 'logins' && segments[2]) {
		return handleLoginStatus(segments[2]);
	}

	throw new ApiError(`Mock endpoint not found: ${method} ${url.pathname}.`, 404);
}
