<script setup lang="ts">
import { Activity, Archive, Bell, CircleDollarSign, KeyRound, Plus, Settings, TrendingUp, Wallet } from 'lucide-vue-next';
import type { PageName } from '../types';

defineProps<{
  activePage: PageName;
  totalRemaining: number;
  todayUsage: number;
  totalProfit: number;
}>();

const emit = defineEmits<{ page: [page: PageName]; add: [] }>();

const items: Array<{ key: PageName; label: string; icon: typeof KeyRound }> = [
  { key: 'tokens', label: '令牌管理', icon: KeyRound },
  { key: 'archived', label: '归档令牌', icon: Archive },
  { key: 'notify', label: '通知设置', icon: Bell },
  { key: 'settings', label: '系统设置', icon: Settings }
];
</script>

<template>
  <aside class="sidebar">
    <div class="brand">
      <span class="brand-mark"><CircleDollarSign :size="21" /></span>
      <span>
        <strong>OR Monitor</strong>
        <small>额度与通知控制台</small>
      </span>
    </div>
    <button class="add-btn" @click="emit('add')"><Plus :size="16" />添加密钥</button>
    <nav class="nav-list" aria-label="主导航">
      <button
        v-for="item in items"
        :key="item.key"
        :class="['nav-item', { active: activePage === item.key }]"
        @click="emit('page', item.key)"
      >
        <component :is="item.icon" :size="16" />
        {{ item.label }}
      </button>
    </nav>
    <div class="sidebar-stats" aria-label="额度摘要">
      <div>
        <Wallet :size="16" />
        <span><span class="stat-label">总剩余额度</span><strong class="stat-value">${{ totalRemaining.toFixed(2) }}</strong></span>
      </div>
      <div>
        <Activity :size="16" />
        <span><span class="stat-label">今日消耗</span><strong class="stat-value">${{ todayUsage.toFixed(2) }}</strong></span>
      </div>
      <div>
        <TrendingUp :size="16" />
        <span><span class="stat-label">总利润</span><strong class="stat-value">¥{{ totalProfit.toFixed(2) }}</strong></span>
      </div>
    </div>
  </aside>
</template>
