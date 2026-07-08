<script setup lang="ts">
import { reactive, watch } from 'vue';
import { Send } from 'lucide-vue-next';
import type { AppSettings, NotifyChannel, NotifyChannels } from '../types';

const props = defineProps<{ settings: AppSettings }>();
const emit = defineEmits<{ save: [settings: AppSettings]; test: [channel: string, config: NotifyChannel] }>();

const local = reactive<Required<Pick<AppSettings, 'enableNotify' | 'notifyInterval' | 'exhaustedThreshold' | 'warningThreshold'>> & { notifyChannels: Required<NotifyChannels> }>({
  enableNotify: false,
  notifyInterval: 60,
  exhaustedThreshold: 2,
  warningThreshold: 20,
  notifyChannels: {
    wechat: { enabled: false, webhook: '' },
    dingtalk: { enabled: false, webhook: '' },
    feishu: { enabled: false, webhook: '' },
    email: { enabled: false, host: '', port: '', from: '', password: '', to: '' }
  }
});

watch(
  () => props.settings,
  (settings) => {
    const channels = settings.notifyChannels || {};
    local.enableNotify = Boolean(settings.enableNotify);
    local.notifyInterval = Number(settings.notifyInterval || 60);
    local.exhaustedThreshold = Number(settings.exhaustedThreshold ?? 2);
    local.warningThreshold = Number(settings.warningThreshold ?? 20);
    local.notifyChannels.wechat = { enabled: Boolean(channels.wechat?.enabled || (settings.enableNotify && settings.webhookUrl)), webhook: channels.wechat?.webhook || settings.webhookUrl || '' };
    local.notifyChannels.dingtalk = { enabled: Boolean(channels.dingtalk?.enabled), webhook: channels.dingtalk?.webhook || '' };
    local.notifyChannels.feishu = { enabled: Boolean(channels.feishu?.enabled), webhook: channels.feishu?.webhook || '' };
    local.notifyChannels.email = {
      enabled: Boolean(channels.email?.enabled),
      host: channels.email?.host || '',
      port: channels.email?.port || '',
      from: channels.email?.from || '',
      password: channels.email?.password || '',
      to: channels.email?.to || ''
    };
  },
  { immediate: true, deep: true }
);
</script>

<template>
  <section class="settings-grid">
    <div class="panel">
      <h2>推送渠道</h2>
      <div class="channel-block">
        <label class="toggle-row"><input v-model="local.notifyChannels.wechat.enabled" type="checkbox" />企业微信</label>
        <div v-if="local.notifyChannels.wechat.enabled" class="inline-form">
          <input v-model="local.notifyChannels.wechat.webhook" placeholder="Webhook URL" />
          <button class="secondary-btn" @click="emit('test', 'wechat', local.notifyChannels.wechat)" type="button"><Send :size="14" />测试</button>
        </div>
      </div>
      <div class="channel-block">
        <label class="toggle-row"><input v-model="local.notifyChannels.dingtalk.enabled" type="checkbox" />钉钉</label>
        <div v-if="local.notifyChannels.dingtalk.enabled" class="inline-form">
          <input v-model="local.notifyChannels.dingtalk.webhook" placeholder="Webhook URL" />
          <button class="secondary-btn" @click="emit('test', 'dingtalk', local.notifyChannels.dingtalk)" type="button"><Send :size="14" />测试</button>
        </div>
      </div>
      <div class="channel-block">
        <label class="toggle-row"><input v-model="local.notifyChannels.feishu.enabled" type="checkbox" />飞书</label>
        <div v-if="local.notifyChannels.feishu.enabled" class="inline-form">
          <input v-model="local.notifyChannels.feishu.webhook" placeholder="Webhook URL" />
          <button class="secondary-btn" @click="emit('test', 'feishu', local.notifyChannels.feishu)" type="button"><Send :size="14" />测试</button>
        </div>
      </div>
      <div class="channel-block">
        <label class="toggle-row"><input v-model="local.notifyChannels.email.enabled" type="checkbox" />邮件</label>
        <div v-if="local.notifyChannels.email.enabled" class="form-grid compact">
          <input v-model="local.notifyChannels.email.host" placeholder="SMTP 主机" />
          <input v-model="local.notifyChannels.email.port" placeholder="端口" />
          <input v-model="local.notifyChannels.email.from" placeholder="发件人" />
          <input v-model="local.notifyChannels.email.to" placeholder="收件人" />
          <input v-model="local.notifyChannels.email.password" type="password" placeholder="密码" />
          <button class="secondary-btn" @click="emit('test', 'email', local.notifyChannels.email)" type="button"><Send :size="14" />测试邮件</button>
        </div>
      </div>
    </div>

    <div class="panel">
      <h2>通知策略</h2>
      <label class="toggle-row"><input v-model="local.enableNotify" type="checkbox" />启用定时通知</label>
      <div class="form-grid compact">
        <label><span>通知间隔（分钟）</span><input v-model.number="local.notifyInterval" min="1" type="number" /></label>
        <label><span>耗尽阈值（$）</span><input v-model.number="local.exhaustedThreshold" min="0" step="0.01" type="number" /></label>
        <label><span>警告阈值（$）</span><input v-model.number="local.warningThreshold" min="0" step="0.01" type="number" /></label>
      </div>
      <button class="primary-btn" type="button" @click="emit('save', JSON.parse(JSON.stringify(local)))">保存通知设置</button>
    </div>
  </section>
</template>
