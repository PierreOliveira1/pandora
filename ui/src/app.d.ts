/// <reference types="vite/client" />

declare global {
	interface ImportMetaEnv {
		readonly VITE_MOCK?: string;
	}

	interface ImportMeta {
		readonly env: ImportMetaEnv;
	}

	namespace App {}
}

export {};
