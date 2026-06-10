@echo off
REM 通卡通构建脚本
cd /d "%~dp0"

REM 1. 构建前端
echo [1/4] 构建前端...
cd cmd\tongkatong-gui\frontend
call npm install --silent 2>nul
call npm run build
cd /d "%~dp0"

REM 2. 复制配置
echo [2/4] 准备配置文件...
if not exist "build_out\config" mkdir "build_out\config"
if not exist "build_out\logs" mkdir "build_out\logs"
copy /Y "config\default.json" "build_out\config\default.json" >nul 2>&1

REM 3. 构建Go
echo [3/4] 构建 tongkatong.exe (headless)...
go build -ldflags="-s -w" -o build_out\tongkatong.exe .\cmd\tongkatong\

echo [3/4] 构建 tongkatong-gui.exe (GUI)...
go build -tags "desktop,production" -ldflags="-H windowsgui -s -w" -o build_out\tongkatong-gui.exe .\cmd\tongkatong-gui\

echo [3/4] 构建 tongkatong-updater.exe...
go build -ldflags="-s -w" -o build_out\tongkatong-updater.exe .\cmd\updater\

REM 4. 完成
echo.
echo === 构建完成 ===
dir build_out\*.exe
echo.
echo 运行: build_out\tongkatong-gui.exe
