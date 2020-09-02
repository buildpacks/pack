@echo off

echo ---- Detect: Internet capable buildpack

curl.exe --silent --head google.com >NUL

if %ERRORLEVEL% equ 0 (
  echo RESULT: Connected to the internet
) else (
  echo RESULT: Disconnected from the internet
)

echo ---- Done
