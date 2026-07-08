<script setup lang="ts">
import { computed, reactive, ref, watch } from 'vue';
import { X } from 'lucide-vue-next';
import type { ApiKey, PriceConfig } from '../types';

const props = defineProps<{ open: boolean; editItem?: ApiKey | null; config: PriceConfig }>();
const emit = defineEmits<{
  close: [];
  submitSingle: [payload: Partial<ApiKey>];
  submitBatch: [text: string, group: string];
}>();

const addMode = ref<'single' | 'batch'>('single');
const customThreshold = ref(false);
const customPrice = ref(false);
const batchText = ref('');
const batchGroup = ref('');
const form = reactive({
  name: '',
  key: '',
  quota: 0,
  group: '',
  exhaustedThreshold: '',
  warningThreshold: '',
  purchaseRate: '',
  sellRate: ''
});

const isEdit = computed(() => Boolean(props.editItem));

watch(
  () => [props.open, props.editItem],
  () => {
    if (!props.open) return;
    addMode.value = 'single';
    const item = props.editItem;
    form.name = item?.name || '';
    form.key = item?.key || '';
    form.quota = Number(item?.quota) || 0;
    form.group = item?.group || '';
    form.exhaustedThreshold = item?.exhaustedThreshold == null ? '' : String(item.exhaustedThreshold);
    form.warningThreshold = item?.warningThreshold == null ? '' : String(item.warningThreshold);
    form.purchaseRate = item?.purchaseRate == null ? '' : String(item.purchaseRate);
    form.sellRate = item?.sellRate == null ? '' : String(item.sellRate);
    customThreshold.value = item?.exhaustedThreshold !== undefined || item?.warningThreshold !== undefined;
    customPrice.value = item?.purchaseRate !== undefined || item?.sellRate !== undefined;
    batchText.value = '';
    batchGroup.value = '';
  },
  { immediate: true }
);

function submit() {
  if (!isEdit.value && addMode.value === 'batch') {
    emit('submitBatch', batchText.value, batchGroup.value.trim() || '默认分组');
    return;
  }
  if (!form.name.trim() || !form.key.trim()) return;

  const payload: Partial<ApiKey> = {
    name: form.name.trim(),
    key: form.key.trim(),
    quota: Number(form.quota) || 0,
    group: form.group.trim() || '默认分组'
  };
  if (customThreshold.value) {
    if (form.exhaustedThreshold !== '') payload.exhaustedThreshold = Number(form.exhaustedThreshold);
    if (form.warningThreshold !== '') payload.warningThreshold = Number(form.warningThreshold);
  } else if (isEdit.value) {
    payload.exhaustedThreshold = null;
    payload.warningThreshold = null;
  }
  if (customPrice.value) {
    if (form.purchaseRate !== '') payload.purchaseRate = Number(form.purchaseRate);
    if (form.sellRate !== '') payload.sellRate = Number(form.sellRate);
  } else if (isEdit.value) {
    payload.purchaseRate = null;
    payload.sellRate = null;
  }
  emit('submitSingle', payload);
}
</script>

<template>
  <div v-if="open" class="modal-backdrop" @click.self="emit('close')">
    <form class="modal" @submit.prevent="submit">
      <header class="modal-header">
        <h2>{{ isEdit ? '编辑密钥' : '添加密钥' }}</h2>
        <button class="icon-btn" type="button" @click="emit('close')"><X :size="18" /></button>
      </header>

      <div v-if="!isEdit" class="segmented">
        <button type="button" :class="{ active: addMode === 'single' }" @click="addMode = 'single'">单个添加</button>
        <button type="button" :class="{ active: addMode === 'batch' }" @click="addMode = 'batch'">批量添加</button>
      </div>

      <template v-if="addMode === 'single'">
        <div class="form-grid">
          <label><span>名称</span><input v-model="form.name" required /></label>
          <label><span>额度</span><input v-model.number="form.quota" type="number" min="0" step="0.01" /></label>
          <label class="wide"><span>密钥</span><input v-model="form.key" required /></label>
          <label class="wide"><span>分组</span><input v-model="form.group" placeholder="默认分组" /></label>
        </div>
        <label class="toggle-row"><input v-model="customThreshold" type="checkbox" />使用密钥专属阈值</label>
        <div v-if="customThreshold" class="form-grid compact">
          <label><span>耗尽阈值</span><input v-model="form.exhaustedThreshold" type="number" min="0" step="0.01" :placeholder="String(config.exhaustedThreshold)" /></label>
          <label><span>警告阈值</span><input v-model="form.warningThreshold" type="number" min="0" step="0.01" :placeholder="String(config.warningThreshold)" /></label>
        </div>
        <label class="toggle-row"><input v-model="customPrice" type="checkbox" />使用密钥专属价格</label>
        <div v-if="customPrice" class="form-grid compact">
          <label><span>收购价</span><input v-model="form.purchaseRate" type="number" min="0" step="0.01" :placeholder="String(config.purchaseRate)" /></label>
          <label><span>售出价</span><input v-model="form.sellRate" type="number" min="0" step="0.01" :placeholder="String(config.sellRate)" /></label>
        </div>
      </template>

      <template v-else>
        <label><span>批量密钥</span><textarea v-model="batchText" rows="9" placeholder="名称,密钥,额度,分组&#10;sk-or-v1-xxx"></textarea></label>
        <label><span>默认分组</span><input v-model="batchGroup" placeholder="默认分组" /></label>
      </template>

      <footer class="modal-footer">
        <button class="secondary-btn" type="button" @click="emit('close')">取消</button>
        <button class="primary-btn" type="submit">{{ isEdit ? '保存修改' : addMode === 'batch' ? '批量添加' : '确认添加' }}</button>
      </footer>
    </form>
  </div>
</template>
