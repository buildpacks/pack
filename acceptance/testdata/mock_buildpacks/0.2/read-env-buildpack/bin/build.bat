@echo off
setlocal EnableDelayedExpansion

set launch_dir=%1
set platform_dir=%2

:: makes a launch layer
if exist %platform_dir%\env\ENV1_CONTENTS (
    echo making env1 layer
    mkdir %launch_dir%\env1-launch-layer
    set /p contents=<%platform_dir%\env\ENV1_CONTENTS
    echo !contents!> %launch_dir%\env1-launch-layer\env1-launch-dep
    mklink env1-launch-dep %launch_dir%\env1-launch-layer\env1-launch-dep
    echo launch = true> %launch_dir%\env1-launch-layer.toml
)

:: makes a launch layer
if exist %platform_dir%\env\ENV2_CONTENTS (
    echo making env2 layer
    mkdir %launch_dir%\env2-launch-layer
    set /p contents=<%platform_dir%\env\ENV2_CONTENTS
    echo !contents!> %launch_dir%\env2-launch-layer\env2-launch-dep
    mklink env2-launch-dep %launch_dir%\env2-launch-layer\env2-launch-dep
    echo launch = true> %launch_dir%\env2-launch-layer.toml
)

echo --- Done
