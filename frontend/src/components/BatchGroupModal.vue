<script setup lang="ts">
import { computed, ref, watch } from 'vue';
import { X } from 'lucide-vue-next';

const props = defineProps<{ open: boolean; count: number; groups: string[] }>();
const emit = defineEmits<{ close: []; confirm: [group: string] }>();

const group = ref('');
const sortedGroups = computed(() => [...new Set(props.groups)].sort());
watch(() => props.open, () => { group.value = ''; });
</script>

<template>
  <div v-if="open" class="modal-backdrop" @click.self="emit('close')">
    <form class="modal small-modal" @submit.prevent="emit('confirm', group.trim() || '默认分组')">
      <header class="modal-header">
        <h2>批量修改分组</h2>
        <button class="icon-btn" type="button" @click="emit('close')"><X :size="18" /></button>
      </header>
      <label><span>目标分组</span><input v-model="group" placeholder="默认分组" /></label>
      <p class="muted">已选择 {{ count }} 个密钥</p>
      <div class="chips">
        <button v-for="item in sortedGroups" :key="item" type="button" @click="group = item">{{ item }}</button>
      </div>
      <footer class="modal-footer">
        <button class="secondary-btn" type="button" @click="emit('close')">取消</button>
        <button class="primary-btn" type="submit">确认修改</button>
      </footer>
    </form>
  </div>
</template>
