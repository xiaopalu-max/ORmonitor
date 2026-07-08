<script setup lang="ts">
import { Archive, Bell, CircleDollarSign, KeyRound, Plus, Settings } from 'lucide-vue-next';
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
    <div class="brand"><CircleDollarSign :size="22" />OR Monitor</div>
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
    <dl class="sidebar-stats">
      <div><dt>总剩余额度</dt><dd>${{ totalRemaining.toFixed(2) }}</dd></div>
      <div><dt>今日消耗</dt><dd>${{ todayUsage.toFixed(2) }}</dd></div>
      <div><dt>总利润</dt><dd>¥{{ totalProfit.toFixed(2) }}</dd></div>
    </dl>
  </aside>
</template>
