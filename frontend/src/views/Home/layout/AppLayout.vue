<template>
  <!-- 顶部标题栏 -->
  <header class="app-header">
    <div class="header-left">
      <!-- macOS 红绿灯窗口控制 -->
      <div class="traffic-lights" @mouseenter="lightsHover = true" @mouseleave="lightsHover = false">
        <button class="light close" data-tip="关闭窗口|⌘W" @click="Window.Close()">
          <svg v-if="lightsHover" width="8" height="8" viewBox="0 0 8 8">
            <line x1="1.5" y1="1.5" x2="6.5" y2="6.5" stroke="rgba(77,0,0,0.85)" stroke-width="1.2" stroke-linecap="round"/>
            <line x1="6.5" y1="1.5" x2="1.5" y2="6.5" stroke="rgba(77,0,0,0.85)" stroke-width="1.2" stroke-linecap="round"/>
          </svg>
        </button>
        <button class="light minimize" data-tip="最小化|⌘M" @click="Window.Minimise()">
          <svg v-if="lightsHover" width="8" height="8" viewBox="0 0 8 8">
            <line x1="1" y1="4" x2="7" y2="4" stroke="rgba(120,70,0,0.85)" stroke-width="1.4" stroke-linecap="round"/>
          </svg>
        </button>
        <button class="light zoom" :data-tip="isMaximised ? '恢复大小' : '缩放'" @click="toggleMaximise">
          <svg v-if="lightsHover" width="8" height="8" viewBox="0 0 8 8">
            <path d="M1.5 4.5 L1.5 6.5 L3.5 6.5 Z" fill="rgba(0,80,0,0.85)"/>
            <path d="M6.5 3.5 L6.5 1.5 L4.5 1.5 Z" fill="rgba(0,80,0,0.85)"/>
          </svg>
        </button>
      </div>
      <div class="logo-container">
        <img src="../../../零启.svg" alt="启SSH Logo" class="logo" />
        <span class="app-name">启SSH</span>
        <span class="version">v{{ version }}</span>
      </div>
    </div>

    <div class="header-right"></div>
  </header>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { Window } from '@wailsio/runtime'
import { GetVersion } from '../../../../bindings/changeme/ssh/greetservice.js'

const isMaximised = ref(false)
const version = ref('0.2.0')
const lightsHover = ref(false)

onMounted(async () => {
  try {
    version.value = await GetVersion()
  } catch (e) {
    console.warn('获取版本失败:', e)
  }
})

// 切换最大化/恢复
async function toggleMaximise() {
  if (isMaximised.value) {
    await Window.Restore()
  } else {
    await Window.Maximise()
  }
}

// 更新最大化按钮状态
async function updateMaximiseButton() {
  isMaximised.value = await Window.IsMaximised()
}

// 监听窗口大小变化
onMounted(() => {
  updateMaximiseButton()

  // 定期检查窗口状态
  setInterval(updateMaximiseButton, 500)
})
</script>

<style scoped>
.app-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  height: 3.25rem;
  padding: 0 1.25rem 0 1rem;
  background: transparent;
  border-bottom: none;
  --wails-draggable: drag;
}

.header-left {
  display: flex;
  align-items: center;
  gap: 1rem;
}

/* ===== macOS 红绿灯 ===== */
.traffic-lights {
  display: flex;
  gap: 8px;
  align-items: center;
  padding: 4px 0;
  --wails-draggable: no-drag;
  -webkit-app-region: no-drag;
}

.light {
  width: 12px;
  height: 12px;
  border-radius: 50%;
  border: 0.5px solid rgba(0, 0, 0, 0.2);
  padding: 0;
  cursor: default;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: filter 0.15s ease;
}

.light svg {
  display: block;
}

.light.close    { background: #ff5f57; }
.light.minimize { background: #febc2e; }
.light.zoom     { background: #28c840; }

.light:active { filter: brightness(0.8); }

/* 窗口失焦时变灰（简化：跟随 body 状态可后续扩展） */

.logo-container {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}

.logo {
  width: 2rem;
  height: 2rem;
  object-fit: contain;
  filter: invert(1);
}

.app-name {
  color: var(--text-primary);
  font-size: 1.3em;
  font-weight: 700;
  letter-spacing: 0.03125rem;
}

.version {
  color: var(--text-muted);
  font-size: 0.7em;
  font-weight: 400;
  padding: 2px 6px;
  background: var(--surface-1);
  border-radius: 4px;
}

.header-right {
  display: flex;
  align-items: center;
}
</style>
