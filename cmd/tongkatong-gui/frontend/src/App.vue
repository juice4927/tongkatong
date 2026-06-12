<script setup>
import { proxyRefs } from 'vue'
import BottomBar from './components/BottomBar.vue'
import StatusBar from './components/StatusBar.vue'
import { useTongkatongApp } from './composables/useTongkatongApp'

const app = proxyRefs(useTongkatongApp())
</script>

<template>
  <div class="app-shell">
    <div class="toast-stack" v-if="app.toastList.length > 0">
      <button
        v-for="toast in app.toastList"
        :key="toast.id"
        class="toast"
        :class="`toast-${toast.type}`"
        @click="app.dismissToast(toast.id)"
      >
        <span class="toast-dot" />
        <span>{{ toast.message }}</span>
      </button>
    </div>

    <StatusBar :app="app" />

    <div class="tab-bar">
      <button
        v-for="tab in app.tabs"
        :key="tab.key"
        class="tab-btn"
        :class="{ active: app.activeTab === tab.key }"
        @click="app.setActiveTab(tab.key)"
      >
        {{ tab.label }}
      </button>
    </div>

    <div class="content dashboard-page" v-if="app.activeTab === 'dashboard'">
      <div class="surface-card hero dashboard-hero">
        <div class="hero-intro">
          <div class="hero-eyebrow">今日概览</div>
          <div class="hero-daywrap">
            <div class="hero-daynum">{{ app.dayNum }}</div>
            <div class="hero-date-col">
              <div class="hero-date">{{ app.dateYM }}</div>
              <div class="hero-sub">{{ app.weekdayStr }} · 保持常驻即可自动执行</div>
            </div>
          </div>
          <div class="hero-badges">
            <span class="hero-badge" :class="app.holidayInfo.is_workday ? 'badge-work' : 'badge-rest'">
              {{ app.holidayInfo.is_workday ? '工作日' : '休息日' }}
            </span>
            <span
              v-if="app.holidayInfo.holiday_name"
              class="hero-badge"
              :class="app.holidayInfo.holiday_name ? 'badge-warn' : 'badge-muted'"
              style="color:#8A4B00"
            >
              {{ app.holidayInfo.holiday_name }}
            </span>
            <span v-else class="hero-badge badge-muted">无节假日</span>
          </div>
          <div class="hero-callout">
            <div class="callout-title">{{ app.dashboardHeadline }}</div>
            <div class="callout-meta">{{ app.dashboardSubline }}</div>
          </div>
        </div>
        <div class="metrics">
          <div class="metric-card" :class="app.holidayInfo.is_workday ? 'metric-ok' : 'metric-warn'">
            <div class="metric-title">今日状态</div>
            <div class="metric-value">{{ app.holidayInfo.is_workday ? '工作日' : '休息日' }}</div>
            <div class="metric-meta">{{ app.holidayInfo.is_workday ? '今天会按规则打卡' : '今天默认不执行自动打卡' }}</div>
          </div>
          <div class="metric-card metric-info">
            <div class="metric-title">下次打卡</div>
            <div class="metric-value">{{ app.nextCheckin || '--' }}</div>
            <div class="metric-meta">{{ app.status.is_running ? '今日排程已生成' : '启动调度后生成今日随机时间' }}</div>
          </div>
          <div class="metric-card metric-info">
            <div class="metric-title">守护状态</div>
            <div class="metric-value">{{ app.guardStatus }}</div>
            <div class="metric-meta">{{ app.guardMeta }}</div>
          </div>
        </div>
      </div>

      <div class="layout-main dashboard-layout">
        <div class="stack-col dashboard-column">
          <div class="surface-card schedule-card">
            <div class="section-title">打卡时间窗口</div>
            <div class="section-sub">今日四个打卡点</div>
            <div class="divider" />
            <div class="grid-2c schedule-grid">
              <div class="metric-card" :class="app.getSlotClass('morning_signin')">
                <div class="metric-title">上午签到</div>
                <div class="metric-value">{{ app.slotTimes.morning_signin || '--:--' }}</div>
                <div class="metric-meta">{{ app.getCheckinRange('morning_signin') }}</div>
              </div>
              <div class="metric-card" :class="app.getSlotClass('morning_signout')">
                <div class="metric-title">上午签退</div>
                <div class="metric-value">{{ app.slotTimes.morning_signout || '--:--' }}</div>
                <div class="metric-meta">{{ app.getCheckinRange('morning_signout') }}</div>
              </div>
              <div class="metric-card" :class="app.getSlotClass('afternoon_signin')">
                <div class="metric-title">下午签到</div>
                <div class="metric-value">{{ app.slotTimes.afternoon_signin || '--:--' }}</div>
                <div class="metric-meta">{{ app.getCheckinRange('afternoon_signin') }}</div>
              </div>
              <div class="metric-card" :class="app.getSlotClass('afternoon_signout')">
                <div class="metric-title">下午签退</div>
                <div class="metric-value">{{ app.slotTimes.afternoon_signout || '--:--' }}</div>
                <div class="metric-meta">{{ app.getCheckinRange('afternoon_signout') }}</div>
              </div>
            </div>
          </div>

          <div class="surface-card results-card">
            <div class="section-title">今日计划</div>
            <div class="section-sub">执行顺序与最新结果</div>
            <div class="divider" />
            <div class="record-list plan-list">
              <div v-for="item in app.dashboardPlanItems" :key="item.key" class="record-item plan-item">
                <div class="plan-main">
                  <span class="plan-label">{{ item.label }}</span>
                  <span class="text-muted">{{ item.range }}</span>
                </div>
                <span class="record-meta">
                  <span class="plan-time">{{ item.planned }}</span>
                  <span v-if="item.success === true" class="tag tag-ok">已完成</span>
                  <span v-else-if="item.success === false" class="tag tag-fail">失败</span>
                  <span v-else class="tag tag-wait">待执行</span>
                  <span v-if="item.timestamp" class="text-muted">{{ item.timestamp }}</span>
                </span>
              </div>
            </div>
          </div>
        </div>

        <div class="stack-col dashboard-column dashboard-side">
          <div class="surface-card summary-card">
            <div class="section-title">运行视图</div>
            <div class="section-sub">当前面板最关键的信息</div>
            <div class="divider" />
            <div class="summary-row"><span class="summary-key">设备目标</span><span class="summary-value">{{ app.mumuCfg.host }}:{{ app.mumuCfg.port }}</span></div>
            <div class="summary-row"><span class="summary-key">应用包名</span><span class="summary-value">{{ app.appCfg.package_name || '(空)' }}</span></div>
            <div class="summary-row"><span class="summary-key">通知状态</span><span class="summary-value">{{ app.notifyCfg.enabled ? '已开启' : '未开启' }}</span></div>
            <div class="summary-row"><span class="summary-key">启动策略</span><span class="summary-value">{{ app.startupSummary }}</span></div>
            <div class="summary-row"><span class="summary-key">节假日规则</span><span class="summary-value">{{ app.holidaySummary }}</span></div>
          </div>

          <div class="soft-note">
            <div class="soft-note-title">当前建议</div>
            <div class="soft-note-body">{{ app.recommendationText }}</div>
          </div>
        </div>
      </div>
    </div>

    <div class="content timeconfig-page" v-if="app.activeTab === 'timeconfig'">
      <div class="timeconfig-layout">
        <div class="surface-card timeconfig-primary">
          <div class="section-head">
            <div class="section-title">打卡时间配置</div>
            <span class="section-chip" :class="{ dirty: app.isSectionDirty('timeconfig') }">{{ app.isSectionDirty('timeconfig') ? '未保存' : '已同步' }}</span>
          </div>
          <div class="section-sub">系统会在每个时间范围内随机选择执行时间</div>
          <div class="divider" />
          <div class="grid-2c time-grid">
            <div v-for="(ct, key) in app.checkinTimes" :key="key" class="time-grid-item">
              <div class="time-card-head">
                <label class="checkbox-label time-label"><input type="checkbox" v-model="ct.enabled"> {{ ct.label }}</label>
                <span class="time-state" :class="ct.enabled ? 'time-state-on' : 'time-state-off'">{{ ct.enabled ? '启用' : '停用' }}</span>
              </div>
              <div class="time-range-row">
                <div class="time-field">
                  <span class="time-hint">开始</span>
                  <input type="time" v-model="ct.time_range[0]" class="time-input" :disabled="!ct.enabled">
                </div>
                <div class="time-field">
                  <span class="time-hint">结束</span>
                  <input type="time" v-model="ct.time_range[1]" class="time-input" :disabled="!ct.enabled">
                </div>
              </div>
            </div>
          </div>
        </div>

        <div class="stack-col timeconfig-side">
          <div class="surface-card">
            <div class="section-title">随机延迟</div>
            <div class="inline-form">
              <span>额外随机延迟:</span>
              <input type="number" class="form-input short-input" v-model.number="app.randomDelay.min" min="0" max="300"> 秒 到
              <input type="number" class="form-input short-input" v-model.number="app.randomDelay.max" min="0" max="300"> 秒
            </div>
          </div>

          <div class="surface-card makeup-card">
            <div class="section-title">补签时间窗口</div>
            <div class="section-sub">启动时若打卡时间已过但仍在窗口内会自动补打；结束大于 23 表示次日</div>
            <div class="divider" />
            <div class="makeup-grid">
              <div class="header">打卡点</div><div class="header">开始时</div><div class="header">开始分</div><div class="header">结束时</div><div class="header">结束分</div>
              <template v-for="(windowCfg, key) in app.makeupWindow" :key="key">
                <span class="makeup-label">{{ windowCfg.label }}</span>
                <input type="number" v-model.number="windowCfg.data[0]" min="0" max="48">
                <input type="number" v-model.number="windowCfg.data[1]" min="0" max="59">
                <input type="number" v-model.number="windowCfg.data[2]" min="0" max="48">
                <input type="number" v-model.number="windowCfg.data[3]" min="0" max="59">
              </template>
            </div>
          </div>
        </div>
      </div>

      <div class="panel-actions timeconfig-actions">
          <button class="btn btn-primary" @click="app.saveTimeConfig" :disabled="app.busy.timeSave || app.busy.save">
            {{ app.busy.timeSave || app.busy.save ? '保存中...' : '保存时间配置' }}
          </button>
          <span class="save-chip" :class="{ dirty: app.hasUnsavedChanges }">{{ app.saveHint }}</span>
      </div>
    </div>

    <div class="content settings-page" v-if="app.activeTab === 'settings'">
      <div class="grid-2c settings-workspace">
        <div class="stack-col settings-column">
          <div class="surface-card">
            <div class="section-head">
              <div class="section-title">MuMu 模拟器设置</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('mumu') }">{{ app.isSectionDirty('mumu') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <div class="form-group">
              <label class="form-label">ADB 路径 <span class="label-muted">留空自动查找</span></label>
              <div class="inline-input">
                <input class="form-input" v-model="app.mumuCfg.adb_path" placeholder="自动查找 MuMu 自带 adb">
                <button class="btn btn-sm" @click="app.browseAdb" :disabled="app.busy.connect">浏览...</button>
              </div>
            </div>
            <div class="form-group">
              <label class="form-label">安装目录 <span class="label-muted">留空自动查找</span></label>
              <div class="inline-input">
                <input class="form-input" v-model="app.mumuCfg.mumu_exe_path" placeholder="自动查找 C/D 盘">
                <button class="btn btn-sm" @click="app.browseMumuDir" :disabled="app.busy.connect">浏览...</button>
              </div>
            </div>
            <div class="inline-form gps-row">
              <span class="strong-label">打卡 GPS:</span>
              <span class="field-hint">纬度</span>
              <input class="form-input coord-input" v-model="app.mumuCfg.gps_latitude" placeholder="31.3191" step="any">
              <span class="field-hint">经度</span>
              <input class="form-input coord-input" v-model="app.mumuCfg.gps_longitude" placeholder="120.5583" step="any">
            </div>
            <div class="inline-form host-row">
              <span class="strong-label host-label">主机地址:</span>
              <input class="form-input host-input" v-model="app.mumuCfg.host">
              <span class="strong-label">端口:</span>
              <input class="form-input port-input" type="number" v-model.number="app.mumuCfg.port" min="1" max="65535">
            </div>
            <div class="panel-actions">
              <button class="btn btn-sm" @click="app.testConnection" :disabled="app.busy.testConnection">
                {{ app.busy.testConnection ? '测试中...' : '测试连接' }}
              </button>
              <button class="btn btn-sm btn-ghost" @click="app.copyDiag">复制诊断信息</button>
              <button class="btn btn-sm btn-ghost" @click="app.exportDiagnostics" :disabled="app.busy.exportDiagnostics">
                {{ app.busy.exportDiagnostics ? '导出中...' : '导出诊断包' }}
              </button>
            </div>
          </div>

          <div class="surface-card">
            <div class="section-head">
              <div class="section-title">交建通 APP 设置</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('app') }">{{ app.isSectionDirty('app') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <div class="form-group">
              <label class="form-label">应用包名</label>
              <div class="inline-input">
                <input class="form-input" v-model="app.appCfg.package_name" placeholder="com.tencent.weworklocal">
                <button class="btn btn-sm btn-accent" @click="app.detectPackage" :disabled="app.busy.detectPackage">
                  {{ app.busy.detectPackage ? '检测中...' : '自动检测' }}
                </button>
              </div>
            </div>
            <div class="pkg-list" v-if="app.showPkgList && app.packages.length > 0">
              <div
                v-for="pkg in app.packages"
                :key="pkg"
                class="pkg-item"
                :class="{ sel: app.appCfg.package_name === pkg }"
                @click="app.appCfg.package_name = pkg; app.showPkgList = false"
              >
                {{ pkg }}
              </div>
            </div>
          </div>
        </div>

        <div class="stack-col settings-column">
          <div class="surface-card">
            <div class="section-head">
              <div class="section-title">节假日设置</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('holiday') }">{{ app.isSectionDirty('holiday') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <label class="checkbox-label"><input type="checkbox" v-model="app.holidayCfg.skip_weekend"> 跳过周末（周六日不打卡）</label>
            <label class="checkbox-label"><input type="checkbox" v-model="app.holidayCfg.skip_holiday"> 跳过法定假日</label>
            <div class="inline-form status-line">
              <span class="field-hint">节假日数据:</span>
              <span>{{ app.holidayDataStatus }}</span>
              <span class="spacer" />
              <button class="btn btn-sm" @click="app.checkHolidayUpdate" :disabled="app.busy.holidayUpdate">
                {{ app.busy.holidayUpdate ? '检查中...' : '检查更新' }}
              </button>
            </div>
            <div class="panel-actions">
              <button class="btn btn-sm" @click="app.showAddDate('workday')">添加工作日</button>
              <button class="btn btn-sm" @click="app.showAddDate('holiday')">添加休息日</button>
              <button class="btn btn-sm btn-ghost" @click="app.removeExtraDate">删除选中</button>
            </div>
            <div class="date-list" v-if="app.extraDates.length > 0">
              <div
                v-for="(item, index) in app.extraDates"
                :key="item.date"
                class="date-item"
                :class="{ sel: app.selExtraDate === index }"
                @click="app.selExtraDate = index"
              >
                <span>{{ item.type === 'workday' ? '工作日' : '休息日' }}: {{ item.date }}</span>
                <span class="text-muted">×</span>
              </div>
            </div>
            <div class="field-note">支持按日期范围批量添加；手动日期优先于系统节假日规则</div>
          </div>

          <div class="surface-card">
            <div class="section-head">
              <div class="section-title">通知设置</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('notify') }">{{ app.isSectionDirty('notify') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <label class="checkbox-label"><input type="checkbox" v-model="app.notifyCfg.enabled"> 启用通知（Server酱推送到微信）</label>
            <div class="form-group">
              <label class="form-label">Server酱 Key</label>
              <div class="pwd-wrap">
                <input class="form-input" :type="app.showWebhook ? 'text' : 'password'" v-model="app.notifyCfg.webhook" placeholder="SCTxxxxxxxx（从 sct.ftqq.com 获取）" :disabled="!app.notifyCfg.enabled">
                <button class="toggle-btn" @click="app.showWebhook = !app.showWebhook" :disabled="!app.notifyCfg.enabled">{{ app.showWebhook ? '隐藏' : '显示' }}</button>
              </div>
            </div>
            <label class="checkbox-label"><input type="checkbox" v-model="app.notifyCfg.verify_tls" :disabled="!app.notifyCfg.enabled"> 验证 HTTPS 证书</label>
            <div class="panel-actions">
              <button class="btn btn-sm btn-ghost" @click="app.sendTestNotification" :disabled="!app.notifyCfg.webhook || app.busy.testNotification">
                {{ app.busy.testNotification ? '发送中...' : '发送测试通知' }}
              </button>
            </div>
          </div>

          <div class="surface-card">
            <div class="section-head">
              <div class="section-title">启动行为</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('startup') }">{{ app.isSectionDirty('startup') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <label class="checkbox-label"><input type="checkbox" v-model="app.advCfg.boot_auto_start"> 开机后自动启动通卡通（后台隐藏启动）</label>
            <label class="checkbox-label"><input type="checkbox" v-model="app.advCfg.auto_connect"> 启动时自动连接设备</label>
            <label class="checkbox-label"><input type="checkbox" v-model="app.advCfg.auto_start"> 连接成功后自动开启打卡</label>
            <label class="checkbox-label"><input type="checkbox" v-model="app.advCfg.keep_alive"> 始终保持运行（常驻守护断线恢复）</label>
            <div class="inline-form dual-inline">
              <span class="strong-label small-label">恢复基准退避:</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_base_backoff" min="0" max="3600">
              <span class="field-hint">秒</span>
              <span class="strong-label small-label">最大退避:</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_max_backoff" min="0" max="86400">
              <span class="field-hint">秒</span>
            </div>
            <div class="inline-form dual-inline">
              <span class="strong-label small-label">连续失败阈值:</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_max_failures" min="0" max="1000">
              <span class="strong-label small-label">触发后暂停:</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_pause_minutes" min="1" max="1440">
              <span class="field-hint">分钟</span>
            </div>
            <label class="checkbox-label"><input type="checkbox" v-model="app.advCfg.recovery_quiet_enabled"> 静默时段暂停自动恢复</label>
            <div class="inline-form dual-inline">
              <span class="strong-label small-label">静默时间:</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_quiet_start" min="0" max="23" :disabled="!app.advCfg.recovery_quiet_enabled">
              <span class="field-hint">点</span>
              <span class="strong-label">到</span>
              <input type="number" class="form-input short-input" v-model.number="app.advCfg.recovery_quiet_end" min="0" max="23" :disabled="!app.advCfg.recovery_quiet_enabled">
              <span class="field-hint">点</span>
            </div>
          </div>
          <div class="surface-card update-card">
            <div class="section-head">
              <div class="section-title">软件更新</div>
              <span class="section-chip" :class="{ dirty: app.isSectionDirty('update') }">{{ app.isSectionDirty('update') ? '未保存' : '已同步' }}</span>
            </div>
            <div class="divider" />
            <div class="update-header">
              <span class="update-version">当前版本: v{{ app.version }}</span>
              <span class="spacer" />
              <span class="field-hint">{{ app.updateStatus }}</span>
            </div>
            <div class="form-group">
              <label class="form-label">清单地址</label>
              <input class="form-input" v-model="app.updateManifestUrl" placeholder="https://raw.githubusercontent.com/.../version.json">
            </div>
            <div class="panel-actions">
              <button class="btn btn-sm" @click="app.checkAppUpdate" :disabled="app.busy.checkUpdate">
                {{ app.busy.checkUpdate ? '检查中...' : '检查更新' }}
              </button>
              <button class="btn btn-sm btn-accent" @click="app.applyUpdate" :disabled="!app.updateAvailable || app.busy.applyUpdate">
                {{ app.busy.applyUpdate ? '更新中...' : '立即更新' }}
              </button>
              <button class="btn btn-sm btn-ghost" @click="app.refreshUpdateStatus" :disabled="app.busy.refreshUpdate">
                {{ app.busy.refreshUpdate ? '刷新中...' : '刷新状态' }}
              </button>
              <label class="checkbox-label"><input type="checkbox" v-model="app.autoCheckUpdate">启动时自动检查</label>
            </div>
            <div class="progress-bar" v-if="app.updateProgress >= 0"><div class="fill" :style="{ width: `${app.updateProgress}%` }" /></div>
            <div class="field-note">{{ app.updateDetail }}</div>
          </div>
        </div>
      </div>

      <div class="panel-actions settings-actions">
        <button class="btn btn-primary" @click="app.saveAllSettings" :disabled="app.busy.save">
          {{ app.busy.save ? '保存中...' : '保存设置' }}
        </button>
        <button class="btn" @click="app.exportConfig" :disabled="app.busy.exportConfig">
          {{ app.busy.exportConfig ? '导出中...' : '导出配置' }}
        </button>
        <button class="btn" @click="app.importConfig" :disabled="app.busy.importConfig">
          {{ app.busy.importConfig ? '导入中...' : '导入配置' }}
        </button>
        <button class="btn btn-ghost" @click="app.resetAll" :disabled="app.busy.reset">
          {{ app.busy.reset ? '恢复中...' : '恢复默认' }}
        </button>
        <span class="save-chip" :class="{ dirty: app.hasUnsavedChanges }">{{ app.saveHint }}</span>
      </div>
    </div>

    <div class="content logs-page" v-if="app.activeTab === 'logs'">
      <div class="log-toolbar">
        <button class="btn btn-sm" @click="app.refreshLog" :disabled="app.busy.refreshLog">
          {{ app.busy.refreshLog ? '刷新中...' : '刷新' }}
        </button>
        <button class="btn btn-sm btn-ghost" @click="app.clearLogs">清屏</button>
        <button class="btn btn-sm btn-ghost" @click="app.autoScroll = !app.autoScroll" :class="{ 'btn-ghost-active': app.autoScroll }">
          自动滚动{{ app.autoScroll ? ' ✓' : '' }}
        </button>
        <button class="btn btn-sm btn-ghost" @click="app.copyLog">复制日志</button>
        <span class="log-summary">{{ app.logSummary }}</span>
      </div>
      <div class="log-filter-bar">
        <input class="form-input log-search" v-model.trim="app.logSearch" placeholder="搜索日志关键字">
        <select class="form-input log-select" v-model="app.logLevel">
          <option value="ALL">全部级别</option>
          <option value="INFO">INFO</option>
          <option value="WARN">WARN</option>
          <option value="ERROR">ERROR</option>
        </select>
        <label class="checkbox-label compact-check"><input type="checkbox" v-model="app.logErrorsOnly"> 仅看异常</label>
      </div>
      <div v-if="app.filteredLogLines.length === 0" class="log-empty">当前筛选条件下没有日志</div>
      <div v-else class="log-viewer" :ref="app.logEl">
        <div
          v-for="(line, index) in app.filteredLogLines"
          :key="`${index}-${line}`"
          class="log-line"
          :class="app.getLogLineClass(line)"
        >
          {{ line }}
        </div>
      </div>
    </div>

    <BottomBar :app="app" />

    <div class="dialog-overlay" v-if="app.dlgShow" @click.self="app.closeDlg">
      <div class="dialog" @click.stop>
        <h3>添加{{ app.dlgType === 'workday' ? '工作日' : '休息日' }}</h3>
        <div class="dlg-body">
          <div class="form-group">
            <label class="form-label">开始日期</label>
            <input type="date" class="form-input" v-model="app.dlgStart" :ref="app.dlgStartEl">
          </div>
          <div class="form-group">
            <label class="form-label">结束日期</label>
            <input type="date" class="form-input" v-model="app.dlgEnd">
          </div>
        </div>
        <div class="dlg-actions">
          <span class="field-hint dlg-tip">Esc 取消 · Enter 确认</span>
          <button class="btn btn-ghost btn-sm" @click="app.closeDlg">取消</button>
          <button class="btn btn-primary btn-sm" @click="app.confirmAddDate">添加</button>
        </div>
      </div>
    </div>

    <div class="dialog-overlay" v-if="app.confirmState.show" @click.self="app.resolveDecision('cancel')">
      <div class="dialog confirm-dialog" :class="`confirm-${app.confirmState.tone}`" @click.stop>
        <h3>{{ app.confirmState.title }}</h3>
        <div class="dlg-body confirm-body">{{ app.confirmState.message }}</div>
        <div class="dlg-actions">
          <button
            v-for="button in app.confirmState.buttons"
            :key="button.key"
            class="btn btn-sm"
            :class="{
              'btn-primary': button.kind === 'primary',
              'btn-danger': button.kind === 'danger',
              'btn-ghost': !button.kind || button.kind === 'ghost'
            }"
            @click="app.resolveDecision(button.key)"
          >
            {{ button.label }}
          </button>
        </div>
      </div>
    </div>
  </div>
</template>
