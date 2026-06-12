<script setup>
defineProps({
  app: {
    type: Object,
    required: true,
  },
})
</script>

<template>
  <div class="btn-bar">
    <button class="btn" @click="app.doConnect" :disabled="app.busy.connect && !app.connecting">
      {{ app.connecting ? '取消连接' : app.busy.connect ? '处理中...' : app.status.is_connected ? '断开连接' : '连接设备' }}
    </button>
    <button class="btn btn-primary" @click="app.toggleScheduler" :disabled="!app.status.is_connected || app.busy.scheduler" v-if="!app.status.is_running">
      {{ app.busy.scheduler ? '处理中...' : '启动调度' }}
    </button>
    <button class="btn btn-danger" @click="app.toggleScheduler" :disabled="app.busy.scheduler" v-else>
      {{ app.busy.scheduler ? '处理中...' : '停止' }}
    </button>
    <button class="btn btn-accent" @click="app.nowCheckin" :disabled="!app.status.is_connected || app.busy.manualCheckin">
      {{ app.busy.manualCheckin ? '执行中...' : '立即打卡' }}
    </button>
    <span class="spacer" />
    <div class="bottom-meta">
      <span class="tiny-dot" :class="app.status.is_connected ? (app.status.is_running ? 'ok' : 'warn') : 'bad'" />
      <span>{{ app.connectionText }} · {{ app.schedulerText }}</span>
    </div>
    <span class="save-chip compact" :class="{ dirty: app.hasUnsavedChanges }">{{ app.saveHint }}</span>
    <button v-if="app.activeTab !== 'settings'" class="btn btn-sm btn-ghost" @click="app.openSettings">设置</button>
    <button class="btn btn-sm btn-ghost btn-exit" @click="app.doQuit">退出</button>
    <button class="btn btn-primary" @click="app.saveAllSettings" :disabled="app.busy.save">
      {{ app.busy.save ? '保存中...' : '保存配置' }}
    </button>
  </div>
</template>
