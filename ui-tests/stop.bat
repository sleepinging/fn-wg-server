@echo off
REM ====================================
REM 安全停止 UI 测试服务器
REM 只杀掉 Mock Server 和 Vite Dev Server，
REM 不会杀掉 pi 或其他 node 进程
REM ====================================

echo === Stopping Mock Server ===
for /f "tokens=2" %%a in ('tasklist /fi "WINDOWTITLE eq MockServer" /fo list ^| findstr "PID:"') do (
    taskkill /PID %%a /F >nul 2>&1
    echo   Killed MockServer (PID %%a)
)

echo === Stopping Vite Dev Server ===
for /f "tokens=2" %%a in ('tasklist /fi "WINDOWTITLE eq ViteDev" /fo list ^| findstr "PID:"') do (
    taskkill /PID %%a /F >nul 2>&1
    echo   Killed ViteDev (PID %%a)
)

REM 备用：按端口杀（如果窗口标题匹配失败）
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":8080.*LISTENING"') do (
    taskkill /PID %%a /F >nul 2>&1
    echo   Killed port 8080 process (PID %%a)
)
for /f "tokens=5" %%a in ('netstat -ano ^| findstr ":5173.*LISTENING"') do (
    taskkill /PID %%a /F >nul 2>&1
    echo   Killed port 5173 process (PID %%a)
)

echo === Done ===
