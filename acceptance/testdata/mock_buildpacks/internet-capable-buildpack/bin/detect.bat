@echo off

echo ---- Detect: Internet capable buildpack

ping -n 1 google.com

if %ERRORLEVEL% equ 0 (
  echo RESULT: Connected to the internet
) else (
  echo RESULT: Disconnected from the internet
)

echo ---- Done
