@echo off

rem 构建脚本
set GO111MODULE=on
set GOPROXY=https://goproxy.io,direct

echo 正在安装依赖...
go mod tidy

if %errorlevel% neq 0 (
    echo 安装依赖失败
    pause
    exit /b %errorlevel%
)

echo 正在构建项目...
go build -o peopleops.exe ./cmd

if %errorlevel% neq 0 (
    echo 构建失败
    pause
    exit /b %errorlevel%
)

echo 构建成功！
pause