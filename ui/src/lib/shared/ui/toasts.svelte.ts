export type ToastTone = 'success' | 'error';

export type ToastItem = {
	id: number;
	message: string;
	tone: ToastTone;
};

export const toasts = $state<ToastItem[]>([]);

let sequence = 0;

export function dismissToast(id: number): void {
	const index = toasts.findIndex((toast) => toast.id === id);
	if (index !== -1) toasts.splice(index, 1);
}

export function showToast(message: string, tone: ToastTone = 'success'): void {
	const id = ++sequence;
	toasts.push({ id, message, tone });
	setTimeout(() => dismissToast(id), 4000);
}
