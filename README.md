# 通卡通 (TongKaTong)

> Go 重写版 · ADB 自动化打卡工具 · 完全开源

基于 Go + Wails + ADB 的自动化打卡工具，通过 MuMu 模拟器驱动目标 App，完成定时打卡、状态守护和异常恢复。

## 项目背景

本项目是从 Python 版（PyQt6 + uiautomator2 + APScheduler）完整重写为 Go 的版本。

**为什么用 Go？**
- 单二进制打包，无 Python 运行时依赖
- goroutine 原生并发，多模拟器并行打卡性能远超 Python 多进程
- 编译期静态类型检查，长期挂机更稳定
- 跨平台编译，分发部署极其简单

## 快速开始

### 前置条件

1. MuMu 模拟器 12 已安装，目标 App 已登录
2. ADB 可用（MuMu 自带或系统安装）

### 编译运行

```bash
# 编译
go build -o tongkatong.exe ./cmd/tongkatong/

# 无界面模式运行
./tongkatong.exe --headless

# 指定配置目录
./tongkatong.exe --config ./config
```

### GUI 模式

```bash
# 需要安装 Wails + Node.js
go install github.com/wailsapp/wails/v2/cmd/wails@latest
cd cmd/tongkatong-gui && wails dev
```

## 项目结构

```
cmd/
  tongkatong/       # 主程序入口（支持 --headless 无界面模式）
  tongkatong-gui/   # Wails GUI 入口
  updater/          # 自动更新器（独立二进制）
internal/
  config/           # 配置管理（内置默认 + default.json + user_config.json）
  models/           # 公共数据类型、版本信息、失败码枚举
  utils/            # 日志、通知、网络探测、记录文件
  adb/              # ADB 设备管理、MuMu 模拟器管理、设备连接池
  automator/        # 自动化核心引擎（XML解析 → 按钮查找 → 导航 → 执行器 → 调度器）
  holiday/          # 节假日判断（内嵌调休数据 + 远程热更新）
  updater/          # 更新系统（manifest / 下载 / 状态管理）
tools/
  build/            # 构建发布工具
```

## 配置

配置文件在 `config/` 目录下，支持三层合并：
1. **内置默认值**（编译时嵌入）
2. **`config/default.json`** — 项目级默认覆盖
3. **`config/user_config.json`** — 用户配置（自动生成）

## 功能特性

| 功能 | 状态 |
|---|---|
| 四时段自动打卡（上午签到/签退、下午签到/签退） | ✅ |
| 随机延迟，避免固定时间执行 | ✅ |
| 周末、节假日自动跳过（含调休数据） | ✅ |
| 补签窗口机制 | ✅ |
| 导航异常自动恢复（返回→回主界面→重启 APP） | ✅ |
| 多级弹窗处理（文本→镜像法→坐标推算） | ✅ |
| 打卡结果验证（关键词/状态变化/时间节点） | ✅ |
| 失败诊断保存（截图 + XML dump） | ✅ |
| GPS 虚拟定位（MuMuManager） | ✅ |
| 网络连通性探测 | ✅ |
| ServerChan 通知推送 + 每日汇总 | ✅ |
| 守护恢复引擎（指数退避 + 静默时段） | ✅ |
| 并行多模拟器支持 | ✅ |
| 跨日自动重调度 | ✅ |
| Wails 桌面 GUI | ✅ |
| 系统托盘常驻 | ✅ |
| 开机自启 | ✅ |
| 自动更新（断点续传 + 流式 SHA256） | ✅ |
| 配置导入/导出 | ✅ |

## 开发

```bash
go test ./...
go vet ./...
go build -o tongkatong.exe ./cmd/tongkatong/
go run ./tools/build/ --version 3.0.0
```

## 许可证

MIT
