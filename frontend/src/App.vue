<template>
  <router-view />
  <Message ref="messageRef" />
  <ConfirmDialog />
</template>

<script setup>
import { ref, onMounted } from 'vue'
import Message from './components/Message.vue'
import ConfirmDialog from './components/ConfirmDialog.vue'
import { setMessageInstance } from './utils/message'
import { Events } from '@wailsio/runtime'

const messageRef = ref(null)

onMounted(() => {
  // 设置全局消息实例
  if (messageRef.value) {
    setMessageInstance(messageRef.value)
  }
  // 通知后端窗口已就绪，可以恢复位置了
  Events.Emit('app:window-ready')
})
</script>

<style>
/* 全局滚动条样式 */
*::-webkit-scrollbar {
  width: 3px;
  height: 3px;
}

*::-webkit-scrollbar-track {
  background: transparent;
}

*::-webkit-scrollbar-thumb {
  background: linear-gradient(180deg, rgba(99, 179, 237, 0.15), rgba(66, 153, 225, 0.3));
  border-radius: 999px;
}

*::-webkit-scrollbar-thumb:hover {
  background: linear-gradient(180deg, rgba(99, 179, 237, 0.3), rgba(66, 153, 225, 0.5));
}

*::-webkit-scrollbar-button {
  display: none;
}
</style>
