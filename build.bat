@echo off
REM 通卡通构建脚本
REM 确保前端已构建
cd /d "%~dp0cmd\tongkatong-gui\frontend"
call npm install --silent 2>nul
call npm run build
cd /d "%~dp0"

echo 构建 tongkatong.exe (headless)...
go build -ldflags="-s -w" -o build_out\tongkatong.exe .\cmd\tongkatong\

echo 构建 tongkatong-gui.exe (GUI)...
go build -tags "desktop,production" -ldflags="-H windowsgui -s -w" -o build_out\tongkatong-gui.exe .\cmd\tongkatong-gui\

echo 构建 tongkatong-updater.exe...
go build -ldflags="-s -w" -o build_out\tongkatong-updater.exe .\cmd\updater\

echo.
echo === 构建完成 ===
dir build_out\*.exe
