<script setup lang="ts">
import { computed, onMounted, onUnmounted, reactive, ref } from 'vue';
import { Archive, Copy, Loader2, RefreshCw, Search, Trash2, X } from 'lucide-vue-next';
import BatchGroupModal from './components/BatchGroupModal.vue';
import KeyCard from './components/KeyCard.vue';
import KeyModal from './components/KeyModal.vue';
import LoginView from './components/LoginView.vue';
import NotifySettings from './components/NotifySettings.vue';
import SidebarNav from './components/SidebarNav.vue';
import SystemSettings from './components/SystemSettings.vue';
import { api, clearSessionId } from './services/api';
import type { ApiKey, AppSettings, KeyUsage, NotifyChannel, PageName, PriceConfig, StatusFilter } from './types';
import { INVALID_GROUP_NAME, INVALID_GROUP_THRESHOLD, keyStatus, parseBatchInput } from './utils/keyMetrics';

const loggedIn = ref(false);
const page = ref<PageName>('tokens');
const loading = ref(false);
const refreshing = ref(false);
const toast = reactive({ visible: false, message: '', type: 'success' as 'success' | 'error' });
const config = reactive<PriceConfig>(loadLocalConfig());
const settings = ref<AppSettings>({});
const keys = ref<ApiKey[]>([]);
const usageById = reactive<Record<string, KeyUsage>>({});
const archivedUsageById = reactive<Record<string, KeyUsage>>({});
const retryCount = reactive<Record<string, number>>({});
const fetching = new Set<string>();
const selected = ref(new Set<string>());
const searchText = ref('');
const statusFilter = ref<StatusFilter>('all');
const keyModalOpen = ref(false);
const editingKey = ref<ApiKey | null>(null);
const batchGroupOpen = ref(false);
let refreshTimer: number | undefined;

function loadLocalConfig(): PriceConfig {
  try {
    const saved = JSON.parse(localStorage.getItem('priceConfig') || '{}');
    return {
      purchaseRate: Number(saved.purchaseRate ?? 3.5),
      sellRate: Number(saved.sellRate ?? 4),
      exhaustedThreshold: Number(saved.exhaustedThreshold ?? 2),
      warningThreshold: Number(saved.warningThreshold ?? 20)
    };
  } catch {
    return { purchaseRate: 3.5, sellRate: 4, exhaustedThreshold: 2, warningThreshold: 20 };
  }
}

function persistConfig() {
  localStorage.setItem('priceConfig', JSON.stringify(config));
}

function showToast(message: string, type: 'success' | 'error' = 'success') {
  toast.message = message;
  toast.type = type;
  toast.visible = true;
  window.setTimeout(() => {
    toast.visible = false;
  }, 3000);
}

function handleUnauthorized(error: unknown) {
  if (error instanceof Error && error.message === 'UNAUTHORIZED') {
    loggedIn.value = false;
    clearSessionId();
    return true;
  }
  return false;
}

async function loadSettings() {
  try {
    const data = await api.settings();
    settings.value = data;
    config.purchaseRate = Number(data.purchaseRate ?? config.purchaseRate);
    config.sellRate = Number(data.sellRate ?? config.sellRate);
    config.exhaustedThreshold = Number(data.exhaustedThreshold ?? config.exhaustedThreshold);
    config.warningThreshold = Number(data.warningThreshold ?? config.warningThreshold);
    persistConfig();
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`加载设置失败：${(error as Error).message}`, 'error');
  }
}

async function loadKeys(fetchBalances = true) {
  loading.value = true;
  try {
    keys.value = await api.keys();
    keys.value.forEach((item) => {
      if ((item.group || '默认分组') === INVALID_GROUP_NAME) {
        usageById[item.id] = { error: '密钥失效' };
      } else if (!usageById[item.id]) {
        usageById[item.id] = { loading: true };
      }
    });
    if (fetchBalances) await refreshBalances();
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`加载密钥失败：${(error as Error).message}`, 'error');
  } finally {
    loading.value = false;
  }
}

async function refreshBalances() {
  const liveKeys = keys.value.filter((item) => !item.archived && (item.group || '默认分组') !== INVALID_GROUP_NAME);
  liveKeys.forEach((item) => {
    usageById[item.id] = usageById[item.id]?.error ? usageById[item.id] : { loading: true };
  });
  await runPool(liveKeys, 8, async (item) => {
    await fetchOpenRouterUsage(item, usageById);
  });
}

async function fetchOpenRouterUsage(item: ApiKey, store: Record<string, KeyUsage>) {
  if (fetching.has(item.id)) return;
  fetching.add(item.id);
  try {
    const result = await fetchWithRetry(item.key);
    store[item.id] = result;
    if (result.error) {
      retryCount[item.id] = (retryCount[item.id] || 0) + 1;
      if (retryCount[item.id] >= INVALID_GROUP_THRESHOLD && !item.archived && (item.group || '默认分组') !== INVALID_GROUP_NAME) {
        await moveToInvalidGroup(item);
      }
    } else {
      delete retryCount[item.id];
    }
  } finally {
    fetching.delete(item.id);
  }
}

async function fetchWithRetry(apiKey: string, attempt = 0): Promise<KeyUsage> {
  const controller = new AbortController();
  const timeoutId = window.setTimeout(() => controller.abort(), 10000);
  try {
    const response = await fetch('https://openrouter.ai/api/v1/key', {
      headers: { Authorization: `Bearer ${apiKey}` },
      signal: controller.signal
    });
    const payload = await response.json();
    if (payload.error) {
      if (attempt < 3) {
        await delay(1000 * (attempt + 1));
        return fetchWithRetry(apiKey, attempt + 1);
      }
      return { error: payload.error.message || '查询失败' };
    }
    return {
      usage: Number(payload.data?.usage || 0),
      usage_daily: Number(payload.data?.usage_daily || 0)
    };
  } catch {
    if (attempt < 3) {
      await delay(1000 * (attempt + 1));
      return fetchWithRetry(apiKey, attempt + 1);
    }
    return { error: 'Network Error' };
  } finally {
    window.clearTimeout(timeoutId);
  }
}

function delay(ms: number) {
  return new Promise((resolve) => window.setTimeout(resolve, ms));
}

async function runPool<T>(items: T[], size: number, worker: (item: T) => Promise<void>) {
  const queue = [...items];
  const workers = Array.from({ length: Math.min(size, queue.length) }, async () => {
    while (queue.length) {
      const next = queue.shift();
      if (next) await worker(next);
    }
  });
  await Promise.allSettled(workers);
}

async function moveToInvalidGroup(item: ApiKey) {
  try {
    const payload = { ...item, group: INVALID_GROUP_NAME };
    await api.updateKey(item.id, payload);
    item.group = INVALID_GROUP_NAME;
    usageById[item.id] = { error: '密钥失效' };
    showToast(`密钥「${item.name}」连续 ${INVALID_GROUP_THRESHOLD} 次获取失败，已移动到${INVALID_GROUP_NAME}`, 'error');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`移动失效分组失败：${(error as Error).message}`, 'error');
  }
}

const activeKeys = computed(() => keys.value.filter((item) => !item.archived));
const archivedKeys = computed(() =>
  keys.value
    .filter((item) => item.archived)
    .sort((a, b) => new Date(b.archivedTime || 0).getTime() - new Date(a.archivedTime || 0).getTime())
);

const filteredKeys = computed(() => {
  const needle = searchText.value.trim().toLowerCase();
  return activeKeys.value.filter((item) => {
    const usage = usageById[item.id] || {};
    const textMatch =
      !needle ||
      item.name.toLowerCase().includes(needle) ||
      item.key.toLowerCase().includes(needle) ||
      (item.group || '默认分组').toLowerCase().includes(needle);
    const statusMatch = statusFilter.value === 'all' || keyStatus(item, usage, config) === statusFilter.value;
    return textMatch && statusMatch;
  });
});

const groupedKeys = computed(() => groupBy(filteredKeys.value));
const groupedArchivedKeys = computed(() => groupBy(archivedKeys.value));
const groupNames = computed(() => Object.keys(groupedKeys.value).sort());
const archivedGroupNames = computed(() => Object.keys(groupedArchivedKeys.value).sort());
const allGroups = computed(() => [...new Set(keys.value.map((item) => item.group || '默认分组'))]);
const selectedCount = computed(() => selected.value.size);
const metrics = computed(() => {
  let totalUsage = 0;
  let totalRemaining = 0;
  let todayUsage = 0;
  let totalProfit = 0;
  let todayProfit = 0;
  activeKeys.value.forEach((item) => {
    const usage = usageById[item.id];
    if (!usage || usage.error || usage.loading) return;
    const total = usage.usage || 0;
    const daily = usage.usage_daily || 0;
    const quota = Number(item.quota) || 0;
    const purchase = Number(item.purchaseRate ?? config.purchaseRate);
    const sell = Number(item.sellRate ?? config.sellRate);
    totalUsage += total;
    todayUsage += daily;
    totalRemaining += quota ? Math.max(0, quota - total) : 0;
    totalProfit += total * (sell - purchase);
    todayProfit += daily * (sell - purchase);
  });
  const rateDiff = config.sellRate - config.purchaseRate;
  const margin = config.purchaseRate > 0 ? (rateDiff / config.purchaseRate) * 100 : 0;
  return { totalUsage, totalRemaining, todayUsage, totalProfit, todayProfit, rateDiff, margin };
});

function groupBy(items: ApiKey[]): Record<string, ApiKey[]> {
  return items.reduce<Record<string, ApiKey[]>>((groups, item) => {
    const group = item.group || '默认分组';
    groups[group] ||= [];
    groups[group].push(item);
    return groups;
  }, {});
}

async function login(username: string, password: string) {
  try {
    await api.login(username, password);
    loggedIn.value = true;
    await bootAuthenticated();
  } catch (error) {
    showToast(`登录失败：${(error as Error).message}`, 'error');
  }
}

async function bootAuthenticated() {
  await loadSettings();
  await loadKeys(true);
  window.clearInterval(refreshTimer);
  refreshTimer = window.setInterval(() => loadKeys(true), 60000);
}

function openAddModal() {
  editingKey.value = null;
  keyModalOpen.value = true;
}

function openEditModal(item: ApiKey) {
  editingKey.value = item;
  keyModalOpen.value = true;
}

async function submitSingle(payload: Partial<ApiKey>) {
  try {
    if (editingKey.value) {
      await api.updateKey(editingKey.value.id, payload);
      showToast('密钥已更新');
    } else {
      await api.createKey(payload as Omit<ApiKey, 'id'>);
      showToast('密钥已添加');
    }
    keyModalOpen.value = false;
    await loadKeys(true);
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`保存失败：${(error as Error).message}`, 'error');
  }
}

async function submitBatch(text: string, group: string) {
  const batch = parseBatchInput(text, group);
  if (!batch.length) {
    showToast('没有有效的密钥信息', 'error');
    return;
  }
  let success = 0;
  let failed = 0;
  for (const item of batch) {
    try {
      await api.createKey(item);
      success += 1;
    } catch {
      failed += 1;
    }
  }
  keyModalOpen.value = false;
  showToast(failed ? `批量添加完成：成功 ${success} 个，失败 ${failed} 个` : `成功添加 ${success} 个密钥`, failed ? 'error' : 'success');
  await loadKeys(true);
}

async function removeKey(id: string) {
  if (!window.confirm('确定要删除这个密钥吗？')) return;
  try {
    await api.deleteKey(id);
    selected.value.delete(id);
    showToast('密钥已删除');
    await loadKeys(false);
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`删除失败：${(error as Error).message}`, 'error');
  }
}

async function archiveKey(id: string) {
  try {
    await api.archiveKey(id);
    selected.value.delete(id);
    showToast('密钥已归档');
    await loadKeys(false);
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`归档失败：${(error as Error).message}`, 'error');
  }
}

async function unarchiveKey(id: string) {
  try {
    await api.unarchiveKey(id);
    showToast('密钥已取消归档');
    await loadKeys(true);
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`取消归档失败：${(error as Error).message}`, 'error');
  }
}

async function refreshArchivedUsage() {
  await loadKeys(false);
  await runPool(archivedKeys.value, 8, async (item) => {
    archivedUsageById[item.id] = { loading: true };
    await fetchOpenRouterUsage(item, archivedUsageById);
  });
}

async function handleRefresh() {
  refreshing.value = true;
  try {
    if (page.value === 'archived') await refreshArchivedUsage();
    else await loadKeys(true);
  } finally {
    refreshing.value = false;
  }
}

function toggleSelect(id: string, checked: boolean) {
  const next = new Set(selected.value);
  if (checked) next.add(id);
  else next.delete(id);
  selected.value = next;
}

function toggleGroupSelection(group: string) {
  const ids = groupedKeys.value[group].map((item) => item.id);
  const next = new Set(selected.value);
  const allSelected = ids.every((id) => next.has(id));
  ids.forEach((id) => (allSelected ? next.delete(id) : next.add(id)));
  selected.value = next;
}

function clearSelection() {
  selected.value = new Set();
}

async function batchDelete() {
  if (!selectedCount.value || !window.confirm(`确定要删除选中的 ${selectedCount.value} 个密钥吗？此操作不可撤销！`)) return;
  let success = 0;
  let failed = 0;
  for (const id of selected.value) {
    try {
      await api.deleteKey(id);
      success += 1;
    } catch {
      failed += 1;
    }
  }
  clearSelection();
  showToast(failed ? `批量删除完成：成功 ${success} 个，失败 ${failed} 个` : `成功删除 ${success} 个密钥`, failed ? 'error' : 'success');
  await loadKeys(false);
}

async function batchArchive() {
  const exhausted = [...selected.value]
    .map((id) => keys.value.find((item) => item.id === id))
    .filter((item): item is ApiKey => Boolean(item))
    .filter((item) => keyStatus(item, usageById[item.id] || {}, config) === 'exhausted');
  if (!exhausted.length) {
    showToast('只能归档耗尽的密钥', 'error');
    return;
  }
  if (!window.confirm(`确定要归档 ${exhausted.length} 个耗尽的密钥吗？`)) return;
  let success = 0;
  for (const item of exhausted) {
    try {
      await api.archiveKey(item.id);
      selected.value.delete(item.id);
      success += 1;
    } catch {
      // Continue with the remaining selection.
    }
  }
  showToast(`成功归档 ${success} 个密钥`);
  await loadKeys(false);
}

async function confirmBatchGroup(group: string) {
  let success = 0;
  let failed = 0;
  for (const id of selected.value) {
    const item = keys.value.find((candidate) => candidate.id === id);
    if (!item) continue;
    try {
      await api.updateKey(id, { ...item, group });
      success += 1;
    } catch {
      failed += 1;
    }
  }
  batchGroupOpen.value = false;
  clearSelection();
  showToast(failed ? `批量修改完成：成功 ${success} 个，失败 ${failed} 个` : `成功修改 ${success} 个密钥的分组`);
  await loadKeys(false);
}

async function copyText(text: string, label = '已复制到剪贴板') {
  try {
    await navigator.clipboard.writeText(text);
    showToast(label);
  } catch {
    showToast('复制失败，请重试', 'error');
  }
}

function copyGroupKeys(group: string) {
  const text = (groupedKeys.value[group] || []).map((item) => item.key).join('\n');
  if (!text) {
    showToast('该分组没有密钥', 'error');
    return;
  }
  copyText(text, `已复制 ${groupedKeys.value[group].length} 个密钥`);
}

async function saveNotifySettings(payload: AppSettings) {
  const hasEnabled = Object.values(payload.notifyChannels || {}).some((channel) => channel?.enabled);
  if (payload.enableNotify && !hasEnabled) {
    showToast('请至少启用一个通知渠道', 'error');
    return;
  }
  try {
    await api.saveSettings(payload);
    await loadSettings();
    showToast('通知设置已保存');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`保存失败：${(error as Error).message}`, 'error');
  }
}

async function testChannel(channel: string, channelConfig: NotifyChannel) {
  try {
    const result = await api.testChannel(channel, channelConfig);
    if (result.success) showToast('测试消息发送成功');
    else showToast(result.error || '发送失败', 'error');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`测试失败：${(error as Error).message}`, 'error');
  }
}

async function savePrice(purchaseRate: number, sellRate: number) {
  try {
    await api.savePrice(purchaseRate, sellRate);
    config.purchaseRate = purchaseRate;
    config.sellRate = sellRate;
    persistConfig();
    showToast('价格配置已保存');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`保存失败：${(error as Error).message}`, 'error');
  }
}

async function saveThresholds(exhaustedThreshold: number, warningThreshold: number) {
  if (exhaustedThreshold < 0 || warningThreshold <= exhaustedThreshold) {
    showToast('警告阈值必须大于耗尽阈值', 'error');
    return;
  }
  try {
    await api.saveSettings({ ...settings.value, exhaustedThreshold, warningThreshold });
    config.exhaustedThreshold = exhaustedThreshold;
    config.warningThreshold = warningThreshold;
    persistConfig();
    await loadSettings();
    showToast('阈值设置已保存');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`保存失败：${(error as Error).message}`, 'error');
  }
}

async function saveTemplate(notifyTemplate: string, keyTemplate: string) {
  try {
    await api.saveTemplate(notifyTemplate, keyTemplate);
    await loadSettings();
    showToast('通知模板已保存');
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`保存失败：${(error as Error).message}`, 'error');
  }
}

async function updateAuth(username: string, password: string) {
  try {
    await api.updateAuth(username, password);
    showToast('账号密码已更新，请重新登录');
    window.setTimeout(() => {
      clearSessionId();
      loggedIn.value = false;
    }, 1500);
  } catch (error) {
    if (!handleUnauthorized(error)) showToast(`更新失败：${(error as Error).message}`, 'error');
  }
}

onMounted(async () => {
  try {
    loggedIn.value = await api.checkAuth();
    if (loggedIn.value) await bootAuthenticated();
  } catch {
    loggedIn.value = false;
  }
});

onUnmounted(() => {
  window.clearInterval(refreshTimer);
});
</script>

<template>
  <LoginView v-if="!loggedIn" @success="login" />

  <div v-else class="app-shell">
    <SidebarNav
      :active-page="page"
      :total-remaining="metrics.totalRemaining"
      :today-usage="metrics.todayUsage"
      :total-profit="metrics.totalProfit"
      @page="page = $event"
      @add="openAddModal"
    />

    <main class="main-content">
      <header class="topbar">
        <div>
          <p class="eyebrow">OpenRouter Key Monitor</p>
          <h1>{{ page === 'tokens' ? '令牌管理' : page === 'archived' ? '归档令牌' : page === 'notify' ? '通知设置' : '系统设置' }}</h1>
        </div>
        <button v-if="page === 'tokens' || page === 'archived'" class="primary-btn" :disabled="refreshing" @click="handleRefresh">
          <Loader2 v-if="refreshing" :size="16" class="spin" />
          <RefreshCw v-else :size="16" />
          刷新
        </button>
      </header>

      <section v-if="page === 'tokens'" class="dashboard">
        <div class="summary-strip">
          <div><span>汇率差价</span><strong>¥{{ metrics.rateDiff.toFixed(2) }} / $1</strong></div>
          <div><span>毛利率</span><strong>{{ metrics.margin.toFixed(2) }}%</strong></div>
          <div><span>总消耗</span><strong>${{ metrics.totalUsage.toFixed(2) }}</strong></div>
          <div><span>今日利润</span><strong>¥{{ metrics.todayProfit.toFixed(2) }}</strong></div>
        </div>

        <div class="toolbar">
          <label class="search-field"><Search :size="16" /><input v-model="searchText" placeholder="搜索名称、分组或密钥" /></label>
          <select v-model="statusFilter">
            <option value="all">全部状态</option>
            <option value="healthy">健康</option>
            <option value="warning">警告</option>
            <option value="exhausted">耗尽</option>
            <option value="error">错误</option>
          </select>
          <button class="secondary-btn" @click="searchText = ''; statusFilter = 'all'"><X :size="14" />清除筛选</button>
        </div>

        <p v-if="loading" class="empty-state">正在加载密钥...</p>
        <p v-else-if="!activeKeys.length" class="empty-state">暂无密钥，请点击左侧“添加密钥”按钮。</p>
        <p v-else-if="!filteredKeys.length" class="empty-state">没有找到匹配的密钥。</p>
        <template v-else>
          <section v-for="group in groupNames" :key="group" class="key-group">
            <header class="group-header">
              <label class="check-row">
                <input
                  type="checkbox"
                  :checked="groupedKeys[group].every((item) => selected.has(item.id))"
                  @change="toggleGroupSelection(group)"
                />
                <strong>{{ group }}</strong>
                <span>{{ groupedKeys[group].length }} 个令牌</span>
              </label>
              <button class="secondary-btn" type="button" @click="copyGroupKeys(group)"><Copy :size="14" />复制所有密钥</button>
            </header>
            <div class="card-grid">
              <KeyCard
                v-for="item in groupedKeys[group]"
                :key="item.id"
                :item="item"
                :usage="usageById[item.id] || { loading: true }"
                :config="config"
                :selected="selected.has(item.id)"
                @select="toggleSelect"
                @copy="copyText"
                @edit="openEditModal"
                @delete="removeKey"
                @archive="archiveKey"
                @unarchive="unarchiveKey"
              />
            </div>
          </section>
        </template>
      </section>

      <section v-if="page === 'archived'" class="dashboard">
        <p v-if="!archivedKeys.length" class="empty-state">暂无归档的密钥。</p>
        <section v-for="group in archivedGroupNames" :key="group" class="key-group">
          <header class="group-header">
            <strong>{{ group }}</strong>
            <span>{{ groupedArchivedKeys[group].length }} 个令牌</span>
          </header>
          <div class="card-grid">
            <KeyCard
              v-for="item in groupedArchivedKeys[group]"
              :key="item.id"
              :item="item"
              :usage="archivedUsageById[item.id] || { loading: false }"
              :config="config"
              archived-view
              @copy="copyText"
              @edit="openEditModal"
              @delete="removeKey"
              @archive="archiveKey"
              @unarchive="unarchiveKey"
            />
          </div>
        </section>
      </section>

      <NotifySettings v-if="page === 'notify'" :settings="settings" @save="saveNotifySettings" @test="testChannel" />
      <SystemSettings
        v-if="page === 'settings'"
        :settings="settings"
        :config="config"
        @save-price="savePrice"
        @save-thresholds="saveThresholds"
        @save-template="saveTemplate"
        @update-auth="updateAuth"
      />
    </main>

    <div v-if="selectedCount" class="batch-bar">
      <span>已选择 {{ selectedCount }} 个密钥</span>
      <button class="secondary-btn" @click="batchGroupOpen = true">修改分组</button>
      <button class="secondary-btn" @click="batchArchive"><Archive :size="14" />归档耗尽</button>
      <button class="danger-btn" @click="batchDelete"><Trash2 :size="14" />删除</button>
      <button class="ghost-btn" @click="clearSelection">清除</button>
    </div>

    <KeyModal
      :open="keyModalOpen"
      :edit-item="editingKey"
      :config="config"
      @close="keyModalOpen = false"
      @submit-single="submitSingle"
      @submit-batch="submitBatch"
    />
    <BatchGroupModal :open="batchGroupOpen" :count="selectedCount" :groups="allGroups" @close="batchGroupOpen = false" @confirm="confirmBatchGroup" />
  </div>

  <div v-if="toast.visible" :class="['toast', toast.type]">{{ toast.message }}</div>
</template>
