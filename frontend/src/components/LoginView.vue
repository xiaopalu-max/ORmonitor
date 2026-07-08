<script setup lang="ts">
import { ref } from 'vue';
import { KeyRound, LogIn } from 'lucide-vue-next';

const emit = defineEmits<{ success: [username: string, password: string] }>();

const username = ref('');
const password = ref('');
const error = ref('');

function submit() {
  if (!username.value.trim() || !password.value) {
    error.value = '请输入账号和密码';
    return;
  }
  error.value = '';
  emit('success', username.value.trim(), password.value);
}
</script>

<template>
  <main class="login-page">
    <form class="login-panel" @submit.prevent="submit">
      <div class="login-mark"><KeyRound :size="24" /></div>
      <h1>OpenRouter 控制台</h1>
      <p>登录后管理 API 密钥额度、归档与通知设置</p>
      <p v-if="error" class="form-error">{{ error }}</p>
      <label>
        <span>用户名</span>
        <input v-model="username" autocomplete="username" placeholder="admin" />
      </label>
      <label>
        <span>密码</span>
        <input v-model="password" type="password" autocomplete="current-password" placeholder="admin123" />
      </label>
      <button class="primary-btn full" type="submit"><LogIn :size="16" />登录</button>
    </form>
  </main>
</template>
