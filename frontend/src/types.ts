export interface ApiKey {
  id: string;
  name: string;
  key: string;
  quota: number;
  group?: string;
  createTime?: string;
  archived?: boolean;
  archivedTime?: string;
  exhaustedThreshold?: number | null;
  warningThreshold?: number | null;
  purchaseRate?: number | null;
  sellRate?: number | null;
}

export interface KeyUsage {
  usage?: number;
  usage_daily?: number;
  loading?: boolean;
  error?: string;
}

export interface NotifyChannel {
  enabled?: boolean;
  webhook?: string;
  host?: string;
  port?: string;
  from?: string;
  password?: string;
  to?: string;
}

export interface NotifyChannels {
  wechat?: NotifyChannel;
  dingtalk?: NotifyChannel;
  feishu?: NotifyChannel;
  email?: NotifyChannel;
}

export interface AppSettings {
  webhookUrl?: string;
  enableNotify?: boolean;
  notifyInterval?: number;
  purchaseRate?: number;
  sellRate?: number;
  exhaustedThreshold?: number;
  warningThreshold?: number;
  notifyChannels?: NotifyChannels;
  notifyTemplate?: string;
  keyTemplate?: string;
}

export interface PriceConfig {
  purchaseRate: number;
  sellRate: number;
  exhaustedThreshold: number;
  warningThreshold: number;
}

export type PageName = 'tokens' | 'archived' | 'notify' | 'settings';
export type StatusFilter = 'all' | 'healthy' | 'warning' | 'exhausted' | 'error';
