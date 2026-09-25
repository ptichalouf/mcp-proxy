export type MCPTransportType = 'stdio' | 'sse' | 'streamable-http' | 'websocket' | 'http';
export type TransportType = MCPTransportType;

export type MCPCategory =
  | 'database'
  | 'developer'
  | 'filesystem'
  | 'cloud'
  | 'search'
  | 'communication'
  | 'productivity'
  | 'monitoring'
  | 'other'
  | string;
export type ServerCategory = MCPCategory;

export interface CatalogEnvVar {
  name?: string;
  key?: string;
  description: string;
  required: boolean;
  isSecret?: boolean;
  secret?: boolean;
  default?: string;
  placeholder?: string;
}

export interface CatalogItem {
  id: string;
  name: string;
  description: string;
  descriptionFr?: string;
  category: MCPCategory;
  vendor?: string;
  author?: string;
  icon?: string;
  verified?: boolean;
  stars?: number;
  downloads?: number;
  license?: string;
  repositoryUrl?: string;
  transportType: MCPTransportType;
  defaultConfig?: {
    command?: string;
    args?: string[];
    url?: string;
    env?: Record<string, string>;
  };
  command?: string;
  args?: string[];
  url?: string;
  env?: CatalogEnvVar[];
  envRequirements?: CatalogEnvVar[];
  docsUrl?: string;
  sourceUrl?: string;
  tags?: string[];
  version?: string;
}

export interface InstalledServer {
  id: string;
  name: string;
  transportType: MCPTransportType;
  command?: string;
  args?: string[];
  url?: string;
  env?: Record<string, string>;
  safeEnv?: Record<string, string>;
  maskedEnvKeys?: string[];
  enabled: boolean;
  status: 'running' | 'stopped' | 'degraded' | 'disabled' | 'error' | 'healthy' | 'unhealthy';
  statusMessage?: string;
  installedVersion?: string;
  updateAvailable?: boolean;
  catalogId?: string;
}

export interface ProxyHealthSummary {
  status: 'ok' | 'degraded' | 'initializing' | 'unavailable' | 'down' | 'ready' | 'error';
  name?: string;
  version?: string;
  uptime?: string;
  activeConnections?: number;
  totalServers?: number;
  activeServers?: number;
  serverCount?: number;
  lastReload?: string;
  unhealthy?: string[];
  configPath?: string;
  configReadable?: boolean;
}

export interface ReloadResult {
  success?: boolean;
  applied?: boolean;
  strategy?: 'in-memory' | 'file-synced' | 'restart-required';
  requiresRestart?: boolean;
  reloadedCount?: number;
  timestamp?: string;
  message?: string;
}

export interface ServerLogEntry {
  timestamp: string;
  level: 'info' | 'warn' | 'error' | 'debug' | string;
  message: string;
  stream?: 'stdout' | 'stderr' | string;
  source?: string;
}
