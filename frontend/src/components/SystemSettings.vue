<script setup lang="ts">
import { reactive, watch } from 'vue';
import type { AppSettings, PriceConfig } from '../types';

const props = defineProps<{ settings: AppSettings; config: PriceConfig }>();
const emit = defineEmits<{
  savePrice: [purchaseRate: number, sellRate: number];
  saveThresholds: [exhaustedThreshold: number, warningThreshold: number];
  saveTemplate: [notifyTemplate: string, keyTemplate: string];
  updateAuth: [username: string, password: string];
}>();

const form = reactive({
  purchaseRate: 3.5,
  sellRate: 4,
  exhaustedThreshold: 2,
  warningThreshold: 20,
  notifyTemplate: '',
  keyTemplate: '',
  username: '',
  password: '',
  confirm: ''
});

watch(
  () => [props.settings, props.config],
  () => {
    form.purchaseRate = Number(props.config.purchaseRate);
    form.sellRate = Number(props.config.sellRate);
    form.exhaustedThreshold = Number(props.config.exhaustedThreshold);
    form.warningThreshold = Number(props.config.warningThreshold);
    form.notifyTemplate = props.settings.notifyTemplate || '';
    form.keyTemplate = props.settings.keyTemplate || '';
  },
  { immediate: true, deep: true }
);

function submitAuth() {
  if (!form.username.trim() || !form.password || form.password !== form.confirm) return;
  emit('updateAuth', form.username.trim(), form.password);
  form.username = '';
  form.password = '';
  form.confirm = '';
}
</script>

<template>
  <section class="settings-grid">
    <div class="panel">
      <h2>价格配置</h2>
      <div class="form-grid compact">
        <label><span>收购价（¥/$）</span><input v-model.number="form.purchaseRate" type="number" min="0" step="0.01" /></label>
        <label><span>售出价（¥/$）</span><input v-model.number="form.sellRate" type="number" min="0" step="0.01" /></label>
      </div>
      <button class="primary-btn" type="button" @click="emit('savePrice', form.purchaseRate, form.sellRate)">保存价格</button>
    </div>

    <div class="panel">
      <h2>余额阈值</h2>
      <div class="form-grid compact">
        <label><span>耗尽阈值（$）</span><input v-model.number="form.exhaustedThreshold" type="number" min="0" step="0.01" /></label>
        <label><span>警告阈值（$）</span><input v-model.number="form.warningThreshold" type="number" min="0" step="0.01" /></label>
      </div>
      <button class="primary-btn" type="button" @click="emit('saveThresholds', form.exhaustedThreshold, form.warningThreshold)">保存阈值</button>
    </div>

    <div class="panel">
      <h2>通知模板</h2>
      <label><span>通知消息模板</span><textarea v-model="form.notifyTemplate" rows="5" /></label>
      <label><span>密钥详情模板</span><textarea v-model="form.keyTemplate" rows="5" /></label>
      <button class="primary-btn" type="button" @click="emit('saveTemplate', form.notifyTemplate, form.keyTemplate)">保存模板</button>
    </div>

    <div class="panel">
      <h2>账号密码</h2>
      <div class="form-grid compact">
        <label><span>新用户名</span><input v-model="form.username" autocomplete="username" /></label>
        <label><span>新密码</span><input v-model="form.password" type="password" autocomplete="new-password" /></label>
        <label><span>确认密码</span><input v-model="form.confirm" type="password" autocomplete="new-password" /></label>
      </div>
      <button class="primary-btn" type="button" :disabled="!form.username || !form.password || form.password !== form.confirm" @click="submitAuth">更新账号</button>
    </div>
  </section>
</template>
