@echo off
setlocal enabledelayedexpansion

set "SERVICE_NAME=harness-docker-runner-svc"

sc query "%SERVICE_NAME%" | find "STATE" | find /i "RUNNING" >nul
if %errorlevel% EQU 0 (
    echo Service "%SERVICE_NAME%" is running.
    echo Stopping service "%SERVICE_NAME%"...
    sc.exe stop "%SERVICE_NAME%"
)

sc query "%SERVICE_NAME%" | find "STATE" | find /i "STOPPED" >nul
if %errorlevel% EQU 0 (
    echo Service "%SERVICE_NAME%" is stopped.
    echo Deleting service "%SERVICE_NAME%"...
    sc.exe delete "%SERVICE_NAME%"
)