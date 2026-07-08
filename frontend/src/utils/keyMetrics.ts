import type { ApiKey, KeyUsage, PriceConfig } from '../types';

export const INVALID_GROUP_NAME = '失效分组';
export const INVALID_GROUP_THRESHOLD = 3;

export function maskKey(key: string): string {
  if (!key) return '';
  if (key.length <= 12) return key;
  return `${key.slice(0, 8)} ••• ${key.slice(-4)}`;
}

export function keyStatus(conf: ApiKey, usage: KeyUsage, config: PriceConfig): 'healthy' | 'warning' | 'exhausted' | 'error' {
  if (usage.error) return 'error';
  const quota = Number(conf.quota) || 0;
  if (!quota) return 'healthy';
  const remaining = Math.max(0, quota - (usage.usage || 0));
  const exhausted = Number(conf.exhaustedThreshold ?? config.exhaustedThreshold ?? 2);
  const warning = Number(conf.warningThreshold ?? config.warningThreshold ?? 20);
  if (remaining < exhausted) return 'exhausted';
  if (remaining < warning) return 'warning';
  return 'healthy';
}

export function effectiveRates(conf: ApiKey, config: PriceConfig) {
  return {
    purchaseRate: Number(conf.purchaseRate ?? config.purchaseRate ?? 3.5),
    sellRate: Number(conf.sellRate ?? config.sellRate ?? 4)
  };
}

export function parseBatchInput(text: string, defaultGroup: string): Array<Omit<ApiKey, 'id'>> {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line, index) => {
      const parts = line.split(',').map((part) => part.trim());
      if (parts.length >= 2) {
        return {
          name: parts[0] || `密钥${index + 1}`,
          key: parts[1],
          quota: Number.parseFloat(parts[2] || '0') || 0,
          group: parts[3] || defaultGroup || '默认分组'
        };
      }
      return {
        name: `密钥${index + 1}`,
        key: parts[0],
        quota: 0,
        group: defaultGroup || '默认分组'
      };
    })
    .filter((item) => item.key);
}
