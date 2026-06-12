import { computed, nextTick, onMounted, onUnmounted, reactive, ref, watch } from 'vue'

export function useTongkatongApp() {
  const version = ref('3.0.0')
  const activeTab = ref('dashboard')
  const tabs = [
    { key: 'dashboard', label: '仪表盘' },
    { key: 'timeconfig', label: '时间配置' },
    { key: 'settings', label: '设置' },
    { key: 'logs', label: '日志' },
  ]

  const status = reactive({ is_connected: false, is_running: false, devices: [] })
  const dailyResults = ref([])
  const logLines = ref([])
  const logSearch = ref('')
  const logLevel = ref('ALL')
  const logErrorsOnly = ref(false)
  const logText = computed(() => logLines.value.join('\n'))
  const filteredLogLines = computed(() => {
    const keyword = logSearch.value.trim().toLowerCase()
    return logLines.value.filter((line) => {
      const upper = line.toUpperCase()
      if (logErrorsOnly.value && !(upper.includes('[WARN]') || upper.includes('[ERROR]') || upper.includes('[FAIL]'))) {
        return false
      }
      if (logLevel.value !== 'ALL' && !upper.includes(`[${logLevel.value}]`)) {
        return false
      }
      if (keyword && !line.toLowerCase().includes(keyword)) {
        return false
      }
      return true
    })
  })
  const filteredLogText = computed(() => filteredLogLines.value.join('\n'))
  const logSummary = computed(() => `共 ${logLines.value.length} 条，当前显示 ${filteredLogLines.value.length} 条`)
  const packages = ref([])
  const connecting = ref(false)
  const refreshing = ref(false)
  const autoScroll = ref(true)
  const logEl = ref(null)
  const nowStr = ref('')
  const dayNum = ref('--')
  const dateYM = ref('')
  const weekdayStr = ref('')
  const holidayInfo = reactive({ is_workday: true, holiday_name: '', date: '' })
  const nextCheckin = ref('')
  const runningJobCount = ref(0)
  const guardStatus = ref('待机')
  const guardMeta = ref('手动启动后进入守护')
  const slotTimes = reactive({
    morning_signin: '',
    morning_signout: '',
    afternoon_signin: '',
    afternoon_signout: '',
  })
  const slotLabels = {
    morning_signin: '上午签到',
    morning_signout: '上午签退',
    afternoon_signin: '下午签到',
    afternoon_signout: '下午签退',
  }
  const holidayDataStatus = ref('加载中...')
  const showPkgList = ref(false)
  const showWebhook = ref(false)
  const updateStatus = ref('未检查')
  const updateAvailable = ref(false)
  const updateProgress = ref(-1)
  const updateDetail = ref('最近更新状态: 暂无')
  const updateManifestUrl = ref('')
  const autoCheckUpdate = ref(false)
  const selExtraDate = ref(-1)

  const checkinTimes = reactive({})
  const mumuCfg = reactive({
    host: '127.0.0.1',
    port: 5555,
    adb_path: '',
    mumu_exe_path: '',
    gps_latitude: 0,
    gps_longitude: 0,
  })
  const appCfg = reactive({ package_name: 'com.tencent.weworklocal', activity: '' })
  const notifyCfg = reactive({ enabled: false, webhook: '', verify_tls: true })
  const holidayCfg = reactive({ skip_weekend: true, skip_holiday: true, extra_workdays: [], extra_holidays: [] })
  const randomDelay = reactive({ min: 1, max: 5 })
  const advCfg = reactive({
    session_ttl_seconds: 300,
    misfire_grace_seconds: 300,
    auto_connect: false,
    auto_start: false,
    boot_auto_start: false,
    keep_alive: true,
    recovery_base_backoff: 5,
    recovery_max_backoff: 300,
    recovery_max_failures: 20,
    recovery_pause_minutes: 30,
    recovery_quiet_enabled: false,
    recovery_quiet_start: 0,
    recovery_quiet_end: 0,
    network_probe: {
      targets: [
        ['223.5.5.5', 53],
        ['114.114.114.114', 53],
        ['1.1.1.1', 53],
        ['8.8.8.8', 53],
      ],
      timeout_seconds: 5,
    },
  })
  const makeupWindow = reactive({
    morning_signin: { label: '上午签到', data: [4, 0, 8, 0] },
    morning_signout: { label: '上午签退', data: [11, 30, 13, 30] },
    afternoon_signin: { label: '下午签到', data: [11, 30, 13, 30] },
    afternoon_signout: { label: '下午签退', data: [17, 0, 28, 0] },
  })
  const extraDates = ref([])

  const dlgShow = ref(false)
  const dlgType = ref('workday')
  const dlgStart = ref('')
  const dlgEnd = ref('')
  const dlgStartEl = ref(null)
  const confirmState = reactive({
    show: false,
    title: '',
    message: '',
    tone: 'info',
    buttons: [],
  })
  const toastList = ref([])
  const busy = reactive({
    connect: false,
    scheduler: false,
    manualCheckin: false,
    save: false,
    timeSave: false,
    holidayUpdate: false,
    detectPackage: false,
    testNotification: false,
    exportDiagnostics: false,
    exportConfig: false,
    importConfig: false,
    reset: false,
    checkUpdate: false,
    applyUpdate: false,
    refreshUpdate: false,
    refreshLog: false,
    testConnection: false,
  })
  const savedSnapshot = ref('')
  const savedSections = ref({})
  const lastSavedAt = ref('')
  let inputFile = null
  let confirmResolver = null

  const connectionText = computed(() => (connecting.value ? '连接中' : status.is_connected ? '设备已连接' : '设备未连接'))
  const schedulerText = computed(() => (status.is_running ? '调度运行中' : '调度未启动'))
  const connectionTone = computed(() => (connecting.value ? 'status-pill-warning' : status.is_connected ? 'status-pill-success' : 'status-pill-danger'))
  const schedulerTone = computed(() => (status.is_running ? 'status-pill-success' : 'status-pill-muted'))
  const guardTone = computed(() => {
    if (guardStatus.value === '恢复中' || guardStatus.value === '等待重试' || guardStatus.value === '已暂停') return 'status-pill-warning'
    if (guardStatus.value === '守护中') return 'status-pill-info'
    return 'status-pill-muted'
  })
  const startupSummary = computed(() => {
    const parts = []
    if (advCfg.boot_auto_start) parts.push('开机自启')
    if (advCfg.auto_connect) parts.push('自动连接')
    if (advCfg.auto_start) parts.push('自动启动调度')
    return parts.length ? parts.join(' / ') : '完全手动'
  })
  const holidaySummary = computed(() => {
    const parts = []
    if (holidayCfg.skip_weekend) parts.push('跳过周末')
    if (holidayCfg.skip_holiday) parts.push('跳过法定假日')
    return parts.length ? parts.join(' / ') : '不跳过'
  })
  const dashboardHeadline = computed(() => {
    if (!status.is_connected) return '先连接设备，系统才会生成今天的真实执行状态。'
    if (!status.is_running) return '设备已经连通，启动调度后会生成今日四个随机打卡点。'
    if (nextCheckin.value) return `下一次打卡：${nextCheckin.value}`
    return holidayInfo.is_workday ? '今日任务已全部生成，请保持程序常驻。' : '今天是休息日，系统默认不会自动打卡。'
  })
  const dashboardSubline = computed(() => {
    if (guardStatus.value === '恢复中' || guardStatus.value === '等待重试' || guardStatus.value === '已暂停') return guardMeta.value
    if (runningJobCount.value > 0) return `今日仍有 ${runningJobCount.value} 个待执行任务，保持通卡通常驻即可自动完成。`
    return notifyCfg.enabled ? '通知已开启，结果和预警会自动推送到微信。' : '如需远程感知打卡结果，建议在设置中开启通知。'
  })
  const recommendationText = computed(() => {
    if (!status.is_connected) return '建议先完成一次设备连接和应用包名确认，确认能够自动打开 APP 后再开启自动启动和开机自启。'
    if (!notifyCfg.enabled) return '建议配置 Server 酱通知，这样打卡失败、每日汇总和预警都能第一时间收到。'
    if (!advCfg.keep_alive) return '当前为手动模式，若你希望像 Python 版那样长期守护，建议开启“始终保持运行”。'
    if (!autoCheckUpdate.value) return '建议开启启动时自动检查更新，这样更新包会在后台提前预下载。'
    return '当前配置已经比较完整，接下来只需保持程序常驻并定期查看日志与诊断信息即可。'
  })
  const dashboardPlanItems = computed(() =>
    Object.entries(slotLabels).map(([key, label]) => {
      const result = dailyResults.value.find((item) => item.job_id === key || item.action_name === label || item.action_name === key)
      return {
        key,
        label,
        planned: slotTimes[key] || '--:--',
        range: getCheckinRange(key),
        success: result ? !!result.success : null,
        timestamp: result?.timestamp || '',
      }
    }),
  )
  const hasUnsavedChanges = computed(() => {
    try {
      return savedSnapshot.value !== serializeConfig()
    } catch {
      return false
    }
  })
  const saveHint = computed(() => {
    if (busy.save || busy.timeSave) return '正在保存配置...'
    if (hasUnsavedChanges.value) return '有未保存的更改'
    if (lastSavedAt.value) return `已保存于 ${lastSavedAt.value}`
    return '配置已同步'
  })
  const dirtySections = computed(() => {
    const current = getSectionSnapshots()
    return Object.fromEntries(
      Object.entries(current).map(([key, value]) => [key, savedSections.value[key] !== value]),
    )
  })

  let clockTmr = null
  let statusTmr = null
  let api = null
  let keydownHandler = null
  let beforeUnloadHandler = null
  let toastSeed = 0
  let connectTimeout = null

  function getApi() {
    if (api) return api
    try {
      if (window.go && window.go.main && window.go.main.App) {
        api = window.go.main.App
      }
    } catch (error) {
      console.warn('Wails API unavailable', error)
    }
    return api
  }

  async function call(method, ...args) {
    const target = getApi()
    if (!target || !target[method]) {
      console.warn('API missing:', method)
      return null
    }
    try {
      return await target[method](...args)
    } catch (error) {
      console.error('Call failed:', method, error)
      return null
    }
  }

  function pushToast(type, message) {
    if (!message) return
    const id = ++toastSeed
    toastList.value.push({ id, type, message })
    window.setTimeout(() => {
      toastList.value = toastList.value.filter((item) => item.id !== id)
    }, 3200)
  }

  function checkinToastType(message) {
    const text = String(message || '')
    if (!text) return 'info'
    if (text.includes('成功')) return 'success'
    if (text.includes('请先') || text.includes('无效') || text.includes('异常') || text.includes('失败')) return 'error'
    return 'info'
  }

  function messageTone(message, successWords = ['成功', '已保存', '已导出', '已导入', '已恢复', '已启动', '已停止', '已发送']) {
    const text = String(message || '')
    if (!text) return 'error'
    if (text.includes('取消')) return 'info'
    const failWords = ['失败', '异常', '请先', '无效', '不可用', '无响应', '校验失败', '启动更新器失败', '检查更新失败']
    if (failWords.some((word) => text.includes(word))) return 'error'
    if (successWords.some((word) => text.includes(word))) return 'success'
    return 'info'
  }

  function finishConnecting() {
    connecting.value = false
    busy.connect = false
    if (connectTimeout) {
      window.clearTimeout(connectTimeout)
      connectTimeout = null
    }
  }

  function dismissToast(id) {
    toastList.value = toastList.value.filter((item) => item.id !== id)
  }

  function requestDecision(options) {
    if (confirmResolver) {
      confirmResolver('cancel')
      confirmResolver = null
    }
    confirmState.title = options.title || '请确认'
    confirmState.message = options.message || ''
    confirmState.tone = options.tone || 'info'
    confirmState.buttons = options.buttons || [{ key: 'confirm', label: '确定', kind: 'primary' }]
    confirmState.show = true
    return new Promise((resolve) => {
      confirmResolver = resolve
    })
  }

  function resolveDecision(key = 'cancel') {
    confirmState.show = false
    const resolver = confirmResolver
    confirmResolver = null
    if (resolver) resolver(key)
  }

  function resetTransientUi() {
    dlgShow.value = false
    dlgType.value = 'workday'
    dlgStart.value = ''
    dlgEnd.value = ''
    confirmState.show = false
    confirmState.title = ''
    confirmState.message = ''
    confirmState.tone = 'info'
    confirmState.buttons = []
    showPkgList.value = false
    showWebhook.value = false
    finishConnecting()
    refreshing.value = false
    Object.keys(busy).forEach((key) => {
      busy[key] = false
    })
  }

  async function runBusy(key, fn) {
    if (busy[key]) return null
    busy[key] = true
    try {
      return await fn()
    } finally {
      busy[key] = false
    }
  }

  function markSaved() {
    savedSnapshot.value = serializeConfig()
    savedSections.value = getSectionSnapshots()
    lastSavedAt.value = new Date().toLocaleTimeString('zh-CN', { hour12: false })
  }

  function serializeConfig() {
    return JSON.stringify(buildConfig())
  }

  function getSectionSnapshots() {
    const config = buildConfig()
    return {
      timeconfig: JSON.stringify({
        checkin: config.checkin,
        random_delay: config.random_delay,
        makeup_window: config.makeup_window,
      }),
      mumu: JSON.stringify(config.mumu),
      app: JSON.stringify(config.app),
      holiday: JSON.stringify(config.holiday),
      notify: JSON.stringify(config.notification),
      startup: JSON.stringify(config.app_state),
      update: JSON.stringify(config.update),
    }
  }

  function isSectionDirty(key) {
    return !!dirtySections.value[key]
  }

  function tickClock() {
    const date = new Date()
    nowStr.value = date.toLocaleTimeString('zh-CN', { hour12: false })
    dayNum.value = date.getDate().toString().padStart(2, '0')
    dateYM.value = `${date.getFullYear()}年${(date.getMonth() + 1).toString().padStart(2, '0')}月`
    weekdayStr.value = ['周日', '周一', '周二', '周三', '周四', '周五', '周六'][date.getDay()]
  }

  function rebuildExtraDates() {
    extraDates.value = []
    for (const date of holidayCfg.extra_workdays || []) {
      extraDates.value.push({ type: 'workday', date })
    }
    for (const date of holidayCfg.extra_holidays || []) {
      extraDates.value.push({ type: 'holiday', date })
    }
    extraDates.value.sort((a, b) => a.date.localeCompare(b.date) || (a.type === 'workday' ? -1 : 1))
  }

  function saveExtraDatesToCfg() {
    holidayCfg.extra_workdays = extraDates.value.filter((item) => item.type === 'workday').map((item) => item.date)
    holidayCfg.extra_holidays = extraDates.value.filter((item) => item.type === 'holiday').map((item) => item.date)
  }

  async function loadConfig() {
    const json = await call('GetConfigJSON')
    if (!json) return
    try {
      const config = typeof json === 'string' ? JSON.parse(json) : json
      if (config.mumu) Object.assign(mumuCfg, config.mumu)
      if (config.app) Object.assign(appCfg, config.app)
      if (config.notification) Object.assign(notifyCfg, config.notification)
      if (config.holiday) {
        Object.assign(holidayCfg, config.holiday)
        rebuildExtraDates()
      }
      if (config.random_delay) {
        randomDelay.min = config.random_delay.min_seconds || 1
        randomDelay.max = config.random_delay.max_seconds || 5
      }
      if (config.advanced) Object.assign(advCfg, config.advanced)
      if (config.app_state) {
        advCfg.auto_connect = !!config.app_state.auto_connect
        advCfg.auto_start = !!config.app_state.auto_start
        advCfg.boot_auto_start = !!config.app_state.boot_auto_start
        advCfg.keep_alive = config.app_state.keep_alive_enabled !== false
        advCfg.recovery_base_backoff = config.app_state.recovery_base_backoff_seconds || 5
        advCfg.recovery_max_backoff = config.app_state.recovery_max_backoff_seconds || 300
        advCfg.recovery_max_failures = config.app_state.recovery_max_failures || 20
        advCfg.recovery_pause_minutes = config.app_state.recovery_pause_minutes_after_max_failures || 30
        advCfg.recovery_quiet_enabled = !!config.app_state.recovery_quiet_hours_enabled
        advCfg.recovery_quiet_start = Number.isFinite(config.app_state.recovery_quiet_start_hour) ? config.app_state.recovery_quiet_start_hour : 0
        advCfg.recovery_quiet_end = Number.isFinite(config.app_state.recovery_quiet_end_hour) ? config.app_state.recovery_quiet_end_hour : 0
      }
      if (config.makeup_window) {
        for (const [key, value] of Object.entries(config.makeup_window)) {
          if (makeupWindow[key] && Array.isArray(value) && value.length === 4) {
            makeupWindow[key].data = [...value]
          }
        }
      }
      if (config.checkin) {
        for (const [key, value] of Object.entries(config.checkin)) {
          checkinTimes[key] = {
            enabled: value.enabled !== false,
            time_range: [...(value.time_range || ['00:00', '00:00'])],
            label: value.label || key,
          }
        }
      }
      if (config.update) {
        updateManifestUrl.value = config.update.manifest_url || ''
        autoCheckUpdate.value = !!config.update.auto_check_on_startup
      }
      markSaved()
    } catch (error) {
      console.error('Config parse:', error)
    }
  }

  function applyDashboard(data) {
    if (!data) return
    status.is_connected = !!data.is_connected
    status.is_running = !!data.is_running
    if (Array.isArray(data.daily_results)) dailyResults.value = data.daily_results
    if (data.holiday) Object.assign(holidayInfo, data.holiday)
    if (data.checkin_times && typeof data.checkin_times === 'object') {
      for (const [key, value] of Object.entries(data.checkin_times)) {
        checkinTimes[key] = {
          enabled: value.enabled !== false,
          time_range: [...(value.time_range || ['00:00', '00:00'])],
          label: value.label || key,
        }
      }
    }
    const planned = data.planned_times && typeof data.planned_times === 'object' ? data.planned_times : {}
    for (const key of Object.keys(slotTimes)) {
      slotTimes[key] = planned[key] || ''
    }
    nextCheckin.value = data.next_checkin || ''
    runningJobCount.value = Number(data.pending_count || 0)
    if (data.devices) status.devices = data.devices
  }

  function fmtShortDateTime(value) {
    if (!value) return ''
    if (typeof value !== 'string') return String(value)
    return value.length >= 16 ? value.slice(5, 16) : value
  }

  function applyGuardState(state) {
    if (!state) return
    if (!state.keep_alive_enabled) {
      guardStatus.value = '未启用'
      guardMeta.value = '不会自动恢复连接或调度'
      return
    }
    if (state.in_progress) {
      guardStatus.value = '恢复中'
      if (state.last_action === 'reconnect' || state.last_action === 'precheck_reconnect') {
        guardMeta.value = '正在重连设备'
      } else if (state.last_reason) {
        guardMeta.value = state.last_reason
      } else {
        guardMeta.value = '正在重启调度'
      }
      return
    }
    if (state.paused_until) {
      guardStatus.value = '已暂停'
      guardMeta.value = `暂停至 ${fmtShortDateTime(state.paused_until)}`
      return
    }
    if (state.next_retry_at) {
      guardStatus.value = '等待重试'
      guardMeta.value = `下次 ${fmtShortDateTime(state.next_retry_at)}${state.fail_count ? ` · 已失败 ${state.fail_count} 次` : ''}`
      return
    }
    if (state.desired_running) {
      guardStatus.value = '守护中'
      guardMeta.value = state.last_reason || (runningJobCount.value > 0 ? `今日剩余 ${runningJobCount.value} 个待执行任务` : '等待下一次执行')
      return
    }
    guardStatus.value = '待机'
    guardMeta.value = '手动启动后进入守护'
  }

  async function refreshAll() {
    if (refreshing.value) return
    refreshing.value = true
    try {
      const [baseStatus, guardState, dashboard] = await Promise.all([
        call('GetStatus'),
        call('GetGuardStatus'),
        call('GetDashboard'),
      ])
      if (baseStatus) Object.assign(status, baseStatus)
      applyGuardState(guardState)
      applyDashboard(dashboard)
      holidayDataStatus.value = holidayInfo.holiday_name ? `已加载 ${holidayInfo.holiday_name}` : '已就绪'
    } finally {
      refreshing.value = false
    }
  }

  function addLog(level, message) {
    const time = new Date().toLocaleTimeString('zh-CN', { hour12: false })
    logLines.value.push(`[${time}] [${level}] ${message}`)
    if (autoScroll.value) {
      nextTick(() => {
        if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
      })
    }
  }

  async function doConnect() {
    if (connecting.value) {
      finishConnecting()
      addLog('WARN', '已取消连接等待')
      pushToast('info', '已取消连接等待')
      const message = await call('DisconnectDevice')
      if (message) addLog('INFO', message)
      await refreshAll()
      return
    }
    if (status.is_connected) {
      await runBusy('connect', async () => {
        const message = await call('DisconnectDevice')
        addLog('INFO', message || '已断开')
        pushToast('info', message || '设备已断开')
        await refreshAll()
      })
      return
    }
    connecting.value = true
    busy.connect = true
    connectTimeout = window.setTimeout(async () => {
      if (!connecting.value) return
      finishConnecting()
      addLog('WARN', '连接等待超时，已恢复按钮状态')
      pushToast('error', '连接超时，已恢复按钮状态，可重试或检查 MuMu/ADB')
      await refreshAll()
    }, 90000)
    pushToast('info', '开始连接设备，首次启动 MuMu 可能需要几十秒')
    addLog('INFO', '正在连接设备...（可能需要60秒启动MuMu）')
    call('ConnectDevice').then((message) => {
      if (message && message !== 'connecting') {
        addLog('INFO', message)
        pushToast(messageTone(message), message)
        finishConnecting()
        refreshAll()
      }
    }).catch((error) => {
      finishConnecting()
      pushToast('error', `连接失败: ${error}`)
    })
  }

  async function toggleScheduler() {
    await runBusy('scheduler', async () => {
      let message
      if (status.is_running) {
        message = await call('StopScheduler')
        addLog('INFO', message || '已停止')
        pushToast('info', message || '调度已停止')
      } else {
        message = await call('StartScheduler')
        addLog('INFO', message || '已启动')
        pushToast(messageTone(message), message || '调度已启动')
      }
      await refreshAll()
    })
  }

  async function doQuit() {
    const canQuit = await confirmUnsavedChanges('退出程序')
    if (!canQuit) return
    const action = await requestDecision({
      title: '确定退出通卡通？',
      message: '如果正在运行打卡任务，建议先停止后再退出。',
      tone: 'warn',
      buttons: [
        { key: 'cancel', label: '取消', kind: 'ghost' },
        { key: 'confirm', label: '退出程序', kind: 'danger' },
      ],
    })
    if (action !== 'confirm') return
    const message = await call('Quit')
    addLog('INFO', message || '正在退出...')
  }

  async function nowCheckin() {
    await runBusy('manualCheckin', async () => {
      const key = Object.keys(checkinTimes).find((item) => checkinTimes[item].enabled) || Object.keys(checkinTimes)[0]
      if (!key) return
      addLog('INFO', `手动执行: ${checkinTimes[key]?.label || key}`)
      pushToast('info', `开始执行 ${checkinTimes[key]?.label || key}`)
      const message = await call('ManualCheckin', key)
      addLog('INFO', message || '完成')
      pushToast(checkinToastType(message), message || '手动打卡已完成')
      await refreshAll()
    })
  }

  async function testConnection() {
    await runBusy('testConnection', async () => {
      addLog('INFO', '测试连接...')
      const message = await call('TestConnection')
      addLog('INFO', message || '无响应')
      pushToast((message || '').includes('成功') ? 'success' : 'info', message || '测试连接已完成')
    })
  }

  async function copyDiag() {
    const text = `host=${mumuCfg.host}\nport=${mumuCfg.port}\nadb=${mumuCfg.adb_path || '(auto)'}\nmumu=${mumuCfg.mumu_exe_path || '(auto)'}\npkg=${appCfg.package_name || '(empty)'}`
    try {
      await navigator.clipboard.writeText(text)
      addLog('INFO', '诊断信息已复制')
      pushToast('success', '诊断信息已复制到剪贴板')
    } catch (error) {
      addLog('WARN', `复制失败: ${error}`)
      pushToast('error', '复制诊断信息失败')
    }
  }

  async function detectPackage() {
    await runBusy('detectPackage', async () => {
      addLog('INFO', '检测已安装包...')
      const result = await call('GetAvailablePackages')
      packages.value = result || []
      showPkgList.value = true
      addLog('INFO', `已获取 ${packages.value.length} 个包`)
      pushToast(packages.value.length > 0 ? 'success' : 'error', packages.value.length > 0 ? `已检测到 ${packages.value.length} 个包` : '未检测到包名，请先连接设备')
    })
  }

  async function checkHolidayUpdate() {
    await runBusy('holidayUpdate', async () => {
      addLog('INFO', '检查节假日更新...')
      const message = await call('CheckHolidayUpdate')
      addLog('INFO', message || '完成')
      pushToast('info', message || '节假日更新检查完成')
    })
  }

  async function sendTestNotification() {
    await runBusy('testNotification', async () => {
      addLog('INFO', '发送测试通知...')
      const message = await call('TestNotification', notifyCfg.webhook, notifyCfg.verify_tls)
      addLog((message || '').includes('已发送') ? 'INFO' : 'WARN', message || '无响应')
      pushToast((message || '').includes('已发送') ? 'success' : 'error', message || '测试通知无响应')
    })
  }

  async function exportDiagnostics() {
    await runBusy('exportDiagnostics', async () => {
      addLog('INFO', '正在导出诊断包...')
      const message = await call('ExportDiagnostics')
      addLog((message || '').includes('已导出') ? 'INFO' : 'WARN', message || '无响应')
      pushToast((message || '').includes('已导出') ? 'success' : 'error', message || '导出诊断包失败')
    })
  }

  function showAddDate(type) {
    const date = new Date().toISOString().slice(0, 10)
    dlgType.value = type
    dlgStart.value = date
    dlgEnd.value = date
    dlgShow.value = true
    nextTick(() => dlgStartEl.value?.focus())
  }

  function closeDlg() {
    dlgShow.value = false
    dlgStart.value = ''
    dlgEnd.value = ''
  }

  function confirmAddDate() {
    if (!dlgStart.value || !dlgEnd.value) {
      addLog('WARN', '请选择日期')
      pushToast('error', '请选择起止日期')
      return
    }
    const start = new Date(dlgStart.value)
    const end = new Date(dlgEnd.value)
    if (Number.isNaN(start.getTime()) || Number.isNaN(end.getTime())) {
      addLog('WARN', '日期格式无效')
      pushToast('error', '日期格式无效')
      return
    }
    for (const date = new Date(start); date <= end; date.setDate(date.getDate() + 1)) {
      const value = date.toISOString().slice(0, 10)
      const found = extraDates.value.findIndex((item) => item.date === value)
      if (found >= 0) extraDates.value[found].type = dlgType.value
      else extraDates.value.push({ type: dlgType.value, date: value })
    }
    extraDates.value.sort((a, b) => a.date.localeCompare(b.date) || (a.type === 'workday' ? -1 : 1))
    saveExtraDatesToCfg()
    pushToast('success', `已添加${dlgType.value === 'workday' ? '工作日' : '休息日'}日期`)
    closeDlg()
  }

  function removeExtraDate() {
    if (selExtraDate.value < 0) {
      addLog('WARN', '请先选中一条日期')
      pushToast('error', '请先选中一条日期')
      return
    }
    extraDates.value.splice(selExtraDate.value, 1)
    selExtraDate.value = -1
    saveExtraDatesToCfg()
    pushToast('success', '已删除选中日期')
  }

  function buildConfig() {
    return {
      mumu: { ...mumuCfg },
      app: { ...appCfg },
      notification: {
        enabled: notifyCfg.enabled,
        webhook: notifyCfg.webhook,
        verify_tls: notifyCfg.verify_tls,
      },
      holiday: {
        skip_weekend: holidayCfg.skip_weekend,
        skip_holiday: holidayCfg.skip_holiday,
        extra_workdays: [...(holidayCfg.extra_workdays || [])],
        extra_holidays: [...(holidayCfg.extra_holidays || [])],
      },
      random_delay: {
        min_seconds: randomDelay.min,
        max_seconds: randomDelay.max,
      },
      advanced: {
        session_ttl_seconds: advCfg.session_ttl_seconds,
        misfire_grace_seconds: advCfg.misfire_grace_seconds,
        network_probe: advCfg.network_probe,
      },
      app_state: {
        auto_connect: advCfg.auto_connect,
        auto_start: advCfg.auto_start,
        boot_auto_start: advCfg.boot_auto_start,
        keep_alive_enabled: advCfg.keep_alive,
        recovery_base_backoff_seconds: advCfg.recovery_base_backoff,
        recovery_max_backoff_seconds: advCfg.recovery_max_backoff,
        recovery_max_failures: advCfg.recovery_max_failures,
        recovery_pause_minutes_after_max_failures: advCfg.recovery_pause_minutes,
        recovery_quiet_hours_enabled: advCfg.recovery_quiet_enabled,
        recovery_quiet_start_hour: advCfg.recovery_quiet_start,
        recovery_quiet_end_hour: advCfg.recovery_quiet_end,
      },
      makeup_window: Object.fromEntries(Object.entries(makeupWindow).map(([key, value]) => [key, [...value.data]])),
      checkin: Object.fromEntries(Object.entries(checkinTimes).map(([key, value]) => [key, {
        enabled: value.enabled,
        time_range: [...value.time_range],
        label: value.label,
      }])),
      update: {
        manifest_url: updateManifestUrl.value,
        auto_check_on_startup: autoCheckUpdate.value,
      },
    }
  }

  async function saveAllSettings() {
    await runBusy('save', async () => {
      const payload = JSON.stringify(buildConfig(), null, 2)
      const result = await call('SaveConfig', payload)
      addLog('INFO', result || '已保存')
      const tone = messageTone(result)
      if (tone === 'success') markSaved()
      pushToast(tone, result || '配置保存无响应')
      await refreshAll()
    })
  }

  async function saveTimeConfig() {
    await runBusy('timeSave', async () => {
      await saveAllSettings()
    })
  }

  async function exportConfig() {
    await runBusy('exportConfig', async () => {
      const data = await call('ExportConfig')
      if (!data) return
      const blob = new Blob([data], { type: 'application/json' })
      const url = URL.createObjectURL(blob)
      const link = document.createElement('a')
      link.href = url
      link.download = 'user_config.json'
      link.click()
      URL.revokeObjectURL(url)
      addLog('INFO', '已导出')
      pushToast('success', '配置文件已导出')
    })
  }

  async function importConfig() {
    if (!inputFile) {
      inputFile = document.createElement('input')
      inputFile.type = 'file'
      inputFile.accept = '.json'
      inputFile.onchange = async (event) => {
        await runBusy('importConfig', async () => {
          const file = event.target.files[0]
          if (!file) return
          const text = await file.text()
          const result = await call('ImportConfig', text)
          addLog('INFO', result || '已导入')
          const tone = messageTone(result)
          pushToast(tone, result || '配置导入无响应')
          if (tone === 'success') {
            await loadConfig()
            await refreshAll()
          }
          inputFile.value = ''
        })
      }
    }
    inputFile.click()
  }

  async function resetAll() {
    const action = await requestDecision({
      title: '确认恢复默认配置？',
      message: '当前页面的改动会被默认值覆盖，此操作无法撤销。',
      tone: 'warn',
      buttons: [
        { key: 'cancel', label: '取消', kind: 'ghost' },
        { key: 'confirm', label: '恢复默认', kind: 'danger' },
      ],
    })
    if (action !== 'confirm') return
    await runBusy('reset', async () => {
      const defaults = await call('GetDefaultConfig')
      if (!defaults) return
      const result = await call('SaveConfig', defaults)
      addLog('INFO', result || '已恢复默认')
      const tone = messageTone(result)
      pushToast(tone, tone === 'success' ? '已恢复默认配置' : (result || '恢复默认无响应'))
      if (tone === 'success') {
        await loadConfig()
        await refreshAll()
      }
    })
  }

  function browseAdb() {
    call('BrowseFile').then((path) => {
      if (!path) return
      mumuCfg.adb_path = path
      addLog('INFO', `已选择: ${path}`)
    })
  }

  function browseMumuDir() {
    call('BrowseDirectory').then((path) => {
      if (!path) return
      mumuCfg.mumu_exe_path = path
      addLog('INFO', `已选择: ${path}`)
    })
  }

  async function checkAppUpdate() {
    await runBusy('checkUpdate', async () => {
      const result = await call('CheckAppUpdate')
      addLog('INFO', `更新: ${result || '无响应'}`)
      updateAvailable.value = false
      updateProgress.value = -1
      try {
        const parsed = JSON.parse(result)
        updateStatus.value = parsed.status || parsed.error || '未检查'
        updateAvailable.value = !!parsed.latest
        updateDetail.value = parsed.notes || parsed.error || result || '暂无'
      } catch {
        updateStatus.value = result || '检查失败'
        updateDetail.value = result || '暂无'
      }
      pushToast('info', updateStatus.value || '更新检查完成')
    })
  }

  async function applyUpdate() {
    await runBusy('applyUpdate', async () => {
      updateStatus.value = '下载中...'
      updateProgress.value = 0
      updateDetail.value = '正在下载更新包...'
      pushToast('info', '开始下载更新包')
      const result = await call('ApplyUpdate')
      addLog('INFO', `更新: ${result || '无响应'}`)
      updateStatus.value = result || '暂无响应'
      pushToast(messageTone(result, ['更新包已下载', '已是最新']), result || '更新无响应')
    })
  }

  async function refreshUpdateStatus() {
    await runBusy('refreshUpdate', async () => {
      const result = await call('RefreshUpdateStatus')
      updateDetail.value = result || '暂无'
      addLog('INFO', `更新状态: ${updateDetail.value}`)
      pushToast('info', '已刷新更新状态')
    })
  }

  async function refreshLog() {
    await runBusy('refreshLog', async () => {
      const content = await call('GetLogContent')
      if (content) logLines.value = content.split('\n')
      if (autoScroll.value) {
        nextTick(() => {
          if (logEl.value) logEl.value.scrollTop = logEl.value.scrollHeight
        })
      }
    })
  }

  function clearLogs() {
    logLines.value = []
    pushToast('info', '日志面板已清空')
  }

  async function copyLog() {
    try {
      await navigator.clipboard.writeText(filteredLogText.value || logText.value)
      addLog('INFO', '日志已复制')
      pushToast('success', '日志已复制到剪贴板')
    } catch (error) {
      addLog('WARN', `复制日志失败: ${error}`)
      pushToast('error', '复制日志失败')
    }
  }

  async function confirmUnsavedChanges(actionLabel = '继续') {
    if (!hasUnsavedChanges.value) return true
    const action = await requestDecision({
      title: '当前有未保存更改',
      message: `你可以先保存再${actionLabel}，也可以放弃当前更改。`,
      tone: 'warn',
      buttons: [
        { key: 'cancel', label: '继续编辑', kind: 'ghost' },
        { key: 'discard', label: '放弃更改', kind: 'ghost' },
        { key: 'save', label: '保存后继续', kind: 'primary' },
      ],
    })
    if (action === 'save') {
      await saveAllSettings()
      return !hasUnsavedChanges.value
    }
    if (action === 'discard') {
      await loadConfig()
      pushToast('info', '已恢复到最近一次保存的配置')
      return true
    }
    return false
  }

  async function setActiveTab(nextTab) {
    if (nextTab === activeTab.value) return
    const settingsTabs = new Set(['settings', 'timeconfig'])
    if (settingsTabs.has(activeTab.value) && !settingsTabs.has(nextTab)) {
      const allowed = await confirmUnsavedChanges('离开当前页面')
      if (!allowed) return
    }
    activeTab.value = nextTab
  }

  async function openSettings() {
    await setActiveTab('settings')
  }

  function handleGlobalKeydown(event) {
    if (confirmState.show && event.key === 'Escape') {
      event.preventDefault()
      resolveDecision('cancel')
      return
    }
    if ((event.ctrlKey || event.metaKey) && event.key.toLowerCase() === 's') {
      event.preventDefault()
      if (activeTab.value === 'timeconfig') {
        saveTimeConfig()
      } else {
        saveAllSettings()
      }
      return
    }
    if (dlgShow.value && event.key === 'Escape') {
      event.preventDefault()
      closeDlg()
      return
    }
    if (dlgShow.value && event.key === 'Enter') {
      const tag = event.target?.tagName
      if (tag !== 'BUTTON') {
        event.preventDefault()
        confirmAddDate()
      }
    }
  }

  function handleBeforeUnload(event) {
    if (!hasUnsavedChanges.value) return
    event.preventDefault()
    event.returnValue = ''
  }

  function getCheckinRange(key) {
    const target = checkinTimes[key]
    return target && target.time_range ? `${target.time_range[0]} ~ ${target.time_range[1]}` : '--'
  }

  function getSlotClass(key) {
    const target = checkinTimes[key]
    return target && target.enabled ? 'metric-ok' : 'metric-info'
  }

  function getLogLineClass(line) {
    const upper = line.toUpperCase()
    if (upper.includes('[ERROR]') || upper.includes('[FAIL]')) return 'log-line-error'
    if (upper.includes('[WARN]')) return 'log-line-warn'
    if (upper.includes('[INFO]')) return 'log-line-info'
    return 'log-line-neutral'
  }

  function setupEvents() {
    try {
      if (!window.runtime || !window.runtime.EventsOn) return
      const on = window.runtime.EventsOn

      on('log', (data) => {
        if (data && data.message) addLog(data.level || 'INFO', data.message)
      })

      on('connect_result', (data) => {
        finishConnecting()
        if (!data) return
        status.is_connected = !!data.success
        addLog(data.success ? 'INFO' : 'WARN', data.message || '')
        if (data.message && data.message.includes('登录未完成')) {
          pushToast('error', '设备已连接，但交建通仍需手动登录确认')
        }
        refreshAll()
      })

      on('connect_progress', (data) => {
        connecting.value = true
        if (data && data.step === 'launching_mumu') addLog('INFO', '正在启动 MuMu 模拟器...')
      })

      on('device_info', (info) => {
        if (info) addLog('INFO', `📱 ${(info.brand || '')} ${(info.model || '')} Android ${info.android_version || '?'}`)
      })

      on('dashboard', (data) => {
        applyDashboard(data)
      })

      on('daily_result', (result) => {
        if (!result) return
        const found = dailyResults.value.findIndex((item) => item.timestamp === result.timestamp)
        if (found >= 0) dailyResults.value[found] = result
        else dailyResults.value.unshift(result)
        if (dailyResults.value.length > 20) {
          dailyResults.value = dailyResults.value.slice(0, 20)
        }
        addLog(result.success ? 'INFO' : 'WARN', `📋 ${result.success ? '✓' : '✗'} ${result.action_name}: ${result.message}`)
      })

      on('guard_status', (state) => {
        applyGuardState(state)
      })

      on('status', (baseStatus) => {
        if (!baseStatus) return
        status.is_connected = !!baseStatus.is_connected
        status.is_running = !!baseStatus.is_running
        if (baseStatus.devices) status.devices = baseStatus.devices
      })

      on('update_status', (update) => {
        if (!update) return
        updateStatus.value = update.status || updateStatus.value
        updateDetail.value = update.detail || updateDetail.value
        updateAvailable.value = !!update.available
        if (!update.available && update.status === '已是最新') updateProgress.value = -1
      })

      on('update_progress', (progress) => {
        if (!progress) return
        updateProgress.value = typeof progress.progress === 'number' ? progress.progress : updateProgress.value
        updateDetail.value = `已下载 ${progress.progress || 0}%`
      })
    } catch (error) {
      console.warn('Events:', error)
    }
  }

  onMounted(async () => {
    resetTransientUi()
    setupEvents()
    keydownHandler = (event) => handleGlobalKeydown(event)
    beforeUnloadHandler = (event) => handleBeforeUnload(event)
    window.addEventListener('keydown', keydownHandler)
    window.addEventListener('beforeunload', beforeUnloadHandler)
    await loadConfig()
    tickClock()
    clockTmr = setInterval(tickClock, 1000)
    await refreshAll()
    statusTmr = setInterval(refreshAll, 30000)
  })

  onUnmounted(() => {
    if (clockTmr) clearInterval(clockTmr)
    if (statusTmr) clearInterval(statusTmr)
    if (connectTimeout) window.clearTimeout(connectTimeout)
    if (keydownHandler) window.removeEventListener('keydown', keydownHandler)
    if (beforeUnloadHandler) window.removeEventListener('beforeunload', beforeUnloadHandler)
  })

  watch(
    () => [notifyCfg.enabled, notifyCfg.webhook],
    () => {
      if (!notifyCfg.enabled) showWebhook.value = false
    },
  )

  return {
    version,
    activeTab,
    tabs,
    status,
    dailyResults,
    logLines,
    logSearch,
    logLevel,
    logErrorsOnly,
    logText,
    filteredLogLines,
    filteredLogText,
    logSummary,
    packages,
    connecting,
    refreshing,
    autoScroll,
    logEl,
    nowStr,
    dayNum,
    dateYM,
    weekdayStr,
    holidayInfo,
    nextCheckin,
    runningJobCount,
    guardStatus,
    guardMeta,
    slotTimes,
    holidayDataStatus,
    showPkgList,
    showWebhook,
    updateStatus,
    updateAvailable,
    updateProgress,
    updateDetail,
    updateManifestUrl,
    autoCheckUpdate,
    selExtraDate,
    checkinTimes,
    mumuCfg,
    appCfg,
    notifyCfg,
    holidayCfg,
    randomDelay,
    advCfg,
    makeupWindow,
    extraDates,
    dlgShow,
    dlgType,
    dlgStart,
    dlgEnd,
    dlgStartEl,
    confirmState,
    toastList,
    busy,
    hasUnsavedChanges,
    saveHint,
    dirtySections,
    connectionText,
    schedulerText,
    connectionTone,
    schedulerTone,
    guardTone,
    startupSummary,
    holidaySummary,
    dashboardHeadline,
    dashboardSubline,
    recommendationText,
    dashboardPlanItems,
    closeDlg,
    doConnect,
    toggleScheduler,
    doQuit,
    nowCheckin,
    testConnection,
    copyDiag,
    detectPackage,
    checkHolidayUpdate,
    sendTestNotification,
    exportDiagnostics,
    showAddDate,
    confirmAddDate,
    removeExtraDate,
    dismissToast,
    resolveDecision,
    confirmUnsavedChanges,
    setActiveTab,
    openSettings,
    saveAllSettings,
    saveTimeConfig,
    exportConfig,
    importConfig,
    resetAll,
    browseAdb,
    browseMumuDir,
    checkAppUpdate,
    applyUpdate,
    refreshUpdateStatus,
    refreshLog,
    clearLogs,
    copyLog,
    refreshAll,
    isSectionDirty,
    getCheckinRange,
    getSlotClass,
    getLogLineClass,
  }
}
