@echo off
REM ====================================
REM UI 自测一键启动脚本
REM 1. 启动 mock 服务器 (端口 8080)
REM 2. 启动 Vite dev server (端口 5173, proxy /api -> 8080)
REM 3. 运行 Playwright 测试
REM
REM 停止服务请运行: stop.bat（不要用 taskkill /IM node.exe！）
REM ====================================

REM 先清理旧进程
call "%~dp0stop.bat" 2>nul

echo === Starting Mock Server ===
cd /d "%~dp0server"
start "MockServer" cmd /c "node server.js"

echo === Starting Vite Dev ===
cd /d "%~dp0..\..\app\ui-src"
start "ViteDev" cmd /c "npx vite --port 5173"

echo === Waiting for servers (8s) ===
timeout /t 8 /nobreak >nul

echo === Running Playwright Tests ===
cd /d "%~dp0tests"
set PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD=1
set CHROMIUM_PATH=C:\Users\saltyfish\AppData\Local\ms-playwright\chromium-1223\chrome-win64\chrome.exe
npx playwright test %*

echo === Cleaning up servers ===
call "%~dp0stop.bat" 2>nul

echo === Done ===
pause
