@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo ====================================
echo   wg-server 一键编译打包
echo ====================================
echo.
echo 当前版本:
grep "^version" pkg\manifest
echo.
echo 自动递增 patch 版本...
echo.
bash build.sh
echo.
echo 清理旧 fpk（仅保留最新2个）...
for /f "skip=2 delims=" %%i in ('dir /b /o-d *.fpk') do del "%%i"
echo.
echo ====================================
echo   完成！
echo ====================================
dir /b /o-d *.fpk
pause
