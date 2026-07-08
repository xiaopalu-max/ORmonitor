<script setup lang="ts">
import { computed } from 'vue';
import { Archive, ArchiveRestore, Copy, Edit3, Loader2, Trash2 } from 'lucide-vue-next';
import type { ApiKey, KeyUsage, PriceConfig } from '../types';
import { effectiveRates, keyStatus, maskKey } from '../utils/keyMetrics';

const props = defineProps<{
  item: ApiKey;
  usage: KeyUsage;
  config: PriceConfig;
  selected?: boolean;
  archivedView?: boolean;
}>();

const emit = defineEmits<{
  select: [id: string, selected: boolean];
  copy: [text: string];
  edit: [item: ApiKey];
  delete: [id: string];
  archive: [id: string];
  unarchive: [id: string];
}>();

const status = computed(() => keyStatus(props.item, props.usage, props.config));
const quota = computed(() => Number(props.item.quota) || 0);
const unlimited = computed(() => !quota.value);
const currentUsage = computed(() => Number(props.usage.usage) || 0);
const daily = computed(() => Number(props.usage.usage_daily) || 0);
const remaining = computed(() => (unlimited.value ? Infinity : Math.max(0, quota.value - currentUsage.value)));
const remainingPercent = computed(() => (unlimited.value ? 100 : Math.max(0, Math.min(100, (remaining.value / quota.value) * 100))));
const rates = computed(() => effectiveRates(props.item, props.config));
const profit = computed(() => currentUsage.value * (rates.value.sellRate - rates.value.purchaseRate));
const archivedTime = computed(() => {
  if (!props.item.archivedTime) return '未知';
  const date = new Date(props.item.archivedTime);
  return Number.isNaN(date.getTime()) ? '未知' : date.toLocaleString('zh-CN');
});
const statusLabel = computed(() => {
  if (props.usage.loading) return '查询中';
  if (props.archivedView) return '已归档';
  return { healthy: '健康', warning: '警告', exhausted: '耗尽', error: '错误' }[status.value];
});
</script>

<template>
  <article :class="['key-card', `status-${status}`, { selected }]">
    <header class="key-card-header">
      <label class="check-row" v-if="!archivedView">
        <input type="checkbox" :checked="selected" @change="emit('select', item.id, ($event.target as HTMLInputElement).checked)" />
        <strong>{{ item.name }}</strong>
      </label>
      <strong v-else>{{ item.name }}</strong>
      <div class="card-actions">
        <span class="badge">{{ statusLabel }}</span>
        <button v-if="!item.archived" class="icon-btn" title="归档" @click="emit('archive', item.id)"><Archive :size="15" /></button>
        <button v-else class="icon-btn" title="取消归档" @click="emit('unarchive', item.id)"><ArchiveRestore :size="15" /></button>
        <button class="icon-btn" title="编辑" @click="emit('edit', item)"><Edit3 :size="15" /></button>
        <button class="icon-btn danger" title="删除" @click="emit('delete', item.id)"><Trash2 :size="15" /></button>
      </div>
    </header>

    <button class="key-mask" @click="emit('copy', item.key)">
      <span>{{ maskKey(item.key) }}</span>
      <Copy :size="13" />
    </button>

    <div v-if="usage.loading" class="loading-line">
      <Loader2 :size="16" class="spin" />
      正在获取额度数据...
    </div>
    <p v-else-if="usage.error" class="error-line">{{ usage.error }}</p>
    <template v-else>
      <div class="progress-meta">
        <span>{{ unlimited ? '无限额度' : `剩 $${remaining.toFixed(2)}` }}</span>
        <span>{{ unlimited ? '—' : `${(100 - remainingPercent).toFixed(1)}%` }}</span>
      </div>
      <div class="progress-track"><div class="progress-bar" :style="{ width: `${remainingPercent}%` }" /></div>
      <dl class="metric-grid">
        <div><dt>总消耗</dt><dd>${{ currentUsage.toFixed(2) }}</dd></div>
        <div><dt>今日</dt><dd>${{ daily.toFixed(3) }}</dd></div>
        <div><dt>额度</dt><dd>{{ unlimited ? '无限' : `$${quota.toFixed(2)}` }}</dd></div>
        <div><dt>利润</dt><dd class="green">¥{{ profit.toFixed(2) }}</dd></div>
        <div><dt>收购价</dt><dd>¥{{ rates.purchaseRate.toFixed(2) }}/$</dd></div>
        <div><dt>售出价</dt><dd>¥{{ rates.sellRate.toFixed(2) }}/$</dd></div>
      </dl>
    </template>

    <p v-if="archivedView" class="archived-time">归档时间：{{ archivedTime }}</p>
  </article>
</template>
