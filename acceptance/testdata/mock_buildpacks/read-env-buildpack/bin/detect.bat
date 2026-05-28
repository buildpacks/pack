@echo off

echo DETECT: Printenv buildpack

set platform_dir=%1

if not exist %platform_dir%\env\DETECT_ENV_BUILDPACK (
    exit 1
)
