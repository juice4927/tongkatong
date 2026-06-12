<script setup>
defineProps({
  app: {
    type: Object,
    required: true,
  },
})
</script>

<template>
  <div class="status-bar">
    <div class="brand-block">
      <div class="brand-title">通卡通</div>
      <div class="brand-sub">自动打卡与守护恢复</div>
    </div>
    <div class="status-pill-wrap">
      <div class="status-pill" :class="app.connectionTone">
        <span class="status-pill-label">连接</span>
        <span class="status-pill-value">{{ app.connectionText }}</span>
      </div>
      <div class="status-pill" :class="app.schedulerTone">
        <span class="status-pill-label">调度</span>
        <span class="status-pill-value">{{ app.schedulerText }}</span>
      </div>
      <div class="status-pill" :class="app.guardTone">
        <span class="status-pill-label">守护</span>
        <span class="status-pill-value">{{ app.guardStatus }}</span>
      </div>
    </div>
    <span class="spacer" />
    <div class="status-pill-wrap">
      <div class="status-pill status-pill-muted">
        <span class="status-pill-label">版本</span>
        <span class="status-pill-value">v{{ app.version }}</span>
      </div>
      <div class="status-pill status-pill-muted">
        <span class="status-pill-label">时间</span>
        <span class="status-pill-value">{{ app.nowStr }}</span>
      </div>
    </div>
  </div>
</template>
