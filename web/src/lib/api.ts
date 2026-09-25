import type {
  CatalogItem,
  InstalledServer,
  ProxyHealthSummary,
  ReloadResult,
  ServerLogEntry,
} from '../types';

export class ApiError extends Error {
  code: string;
  status: number;

  constructor(message: string, code: string = 'GENERIC_ERROR', status: number = 500) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
  }
}

async function handleResponse<T>(res: Response): Promise<T> {
  if (!res.ok) {
    let errorMsg = `HTTP Error ${res.status}`;
    let errorCode = 'GENERIC_ERROR';
    try {
      const data = await res.json();
      if (data.message) errorMsg = data.message;
      if (data.code) errorCode = data.code;
      if (data.error) errorMsg = data.error;
    } catch {
      // Ignore JSON parse error on non-json error responses
    }
    throw new ApiError(errorMsg, errorCode, res.status);
  }
  return res.json() as Promise<T>;
}

export const api = {
  async getCatalog(query?: string, category?: string): Promise<CatalogItem[]> {
    const params = new URLSearchParams();
    if (query) params.set('q', query);
    if (category && category !== 'all') params.set('category', category);
    const qs = params.toString() ? `?${params.toString()}` : '';
    const res = await fetch(`/api/mcp/catalog${qs}`);
    const data = await handleResponse<{ items: CatalogItem[] }>(res);
    return data.items || [];
  },

  async getInstalledServers(): Promise<InstalledServer[]> {
    const res = await fetch('/api/mcp/installed');
    const data = await handleResponse<{ servers: InstalledServer[] }>(res);
    return data.servers || [];
  },

  async getProxyHealth(): Promise<ProxyHealthSummary> {
    const res = await fetch('/api/mcp/proxy/health');
    return handleResponse<ProxyHealthSummary>(res);
  },

  async reloadProxy(): Promise<ReloadResult> {
    const res = await fetch('/api/mcp/proxy/reload', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
    });
    return handleResponse<ReloadResult>(res);
  },

  async installServer(data: {
    catalogId?: string;
    name: string;
    transportType?: string;
    command?: string;
    args?: string[];
    url?: string;
    env?: Record<string, string>;
  }): Promise<{ server: InstalledServer; reload: ReloadResult }> {
    const res = await fetch('/api/mcp/install', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return handleResponse<{ server: InstalledServer; reload: ReloadResult }>(res);
  },

  async updateServer(
    id: string,
    data: {
      name?: string;
      enabled?: boolean;
      env?: Record<string, string>;
      args?: string[];
      command?: string;
      url?: string;
    },
  ): Promise<{ server: InstalledServer; reload: ReloadResult }> {
    const res = await fetch(`/api/mcp/servers/${encodeURIComponent(id)}`, {
      method: 'PATCH',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(data),
    });
    return handleResponse<{ server: InstalledServer; reload: ReloadResult }>(res);
  },

  async uninstallServer(id: string): Promise<{ success: boolean; reload: ReloadResult }> {
    const res = await fetch(`/api/mcp/servers/${encodeURIComponent(id)}`, {
      method: 'DELETE',
    });
    return handleResponse<{ success: boolean; reload: ReloadResult }>(res);
  },

  async getServerLogs(id: string): Promise<ServerLogEntry[]> {
    const res = await fetch(`/api/mcp/servers/${encodeURIComponent(id)}/logs`);
    const data = await handleResponse<{ logs: ServerLogEntry[] }>(res);
    return data.logs || [];
  },
};
