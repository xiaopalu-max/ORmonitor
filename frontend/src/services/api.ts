import type { ApiKey, AppSettings, NotifyChannel } from '../types';

export function getSessionId(): string {
  return localStorage.getItem('sessionId') || '';
}

export function setSessionId(sessionId: string): void {
  localStorage.setItem('sessionId', sessionId);
}

export function clearSessionId(): void {
  localStorage.removeItem('sessionId');
}

function withSession(path: string): string {
  const url = new URL(path, window.location.origin);
  const sessionId = getSessionId();
  if (sessionId) url.searchParams.set('sessionId', sessionId);
  return `${url.pathname}${url.search}`;
}

async function request<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers);
  if (!headers.has('Content-Type') && init.body) headers.set('Content-Type', 'application/json');
  const sessionId = getSessionId();
  if (sessionId) headers.set('x-session-id', sessionId);

  const response = await fetch(withSession(path), { ...init, headers });
  const contentType = response.headers.get('content-type') || '';
  const payload = contentType.includes('application/json') ? await response.json() : await response.text();

  if (response.status === 401) {
    clearSessionId();
    throw new Error('UNAUTHORIZED');
  }
  if (!response.ok) {
    const message = typeof payload === 'object' && payload && 'error' in payload ? String(payload.error) : response.statusText;
    throw new Error(message || '请求失败');
  }
  return payload as T;
}

export const api = {
  async login(username: string, password: string): Promise<string> {
    const result = await request<{ success: boolean; sessionId: string; error?: string }>('/api/auth/login', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    });
    if (!result.success || !result.sessionId) throw new Error(result.error || '登录失败');
    setSessionId(result.sessionId);
    return result.sessionId;
  },

  async checkAuth(): Promise<boolean> {
    if (!getSessionId()) return false;
    const result = await request<{ loggedIn: boolean }>('/api/auth/check');
    return Boolean(result.loggedIn);
  },

  async logout(): Promise<void> {
    await request('/api/auth/logout', { method: 'POST' });
    clearSessionId();
  },

  updateAuth(username: string, password: string) {
    return request<{ success: boolean }>('/api/auth/update', {
      method: 'POST',
      body: JSON.stringify({ username, password })
    });
  },

  keys() {
    return request<ApiKey[]>('/api/keys');
  },

  createKey(key: Omit<ApiKey, 'id'>) {
    return request<ApiKey>('/api/keys', { method: 'POST', body: JSON.stringify(key) });
  },

  updateKey(id: string, key: Partial<ApiKey>) {
    return request<ApiKey>(`/api/keys/${id}`, { method: 'PUT', body: JSON.stringify(key) });
  },

  deleteKey(id: string) {
    return request<{ success: boolean }>(`/api/keys/${id}`, { method: 'DELETE' });
  },

  archiveKey(id: string) {
    return request<{ success: boolean }>(`/api/keys/${id}/archive`, { method: 'POST' });
  },

  unarchiveKey(id: string) {
    return request<{ success: boolean }>(`/api/keys/${id}/archive`, { method: 'DELETE' });
  },

  settings() {
    return request<AppSettings>('/api/settings');
  },

  saveSettings(settings: AppSettings) {
    return request<{ success: boolean; settings?: AppSettings }>('/api/settings', {
      method: 'POST',
      body: JSON.stringify(settings)
    });
  },

  savePrice(purchaseRate: number, sellRate: number) {
    return request<{ success: boolean }>('/api/settings/price', {
      method: 'POST',
      body: JSON.stringify({ purchaseRate, sellRate })
    });
  },

  saveTemplate(notifyTemplate: string, keyTemplate: string) {
    return request<{ success: boolean }>('/api/settings/template', {
      method: 'POST',
      body: JSON.stringify({ notifyTemplate, keyTemplate })
    });
  },

  testChannel(channel: string, config: NotifyChannel) {
    const path = `/api/test-channel?channel=${encodeURIComponent(channel)}`;
    return request<{ success: boolean; error?: string }>(path, {
      method: 'POST',
      body: JSON.stringify(config)
    });
  }
};
