@echo off

rem 运行脚本
set GO111MODULE=on
set GOPROXY=https://goproxy.io,direct

echo 正在启动服务...
go run ./cmd

pause