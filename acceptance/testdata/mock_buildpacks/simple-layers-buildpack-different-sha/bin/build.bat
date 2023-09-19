@echo off
echo --- Build: Simple Layers Different Sha Buildpack

set launch_dir=%1

:: makes a launch layer
echo making launch layer %launch_dir%\launch-layer
mkdir %launch_dir%\launch-layer
echo Launch Dep Contents > "%launch_dir%\launch-layer\launch-dep
mklink /j launch-deps %launch_dir%\launch-layer
echo [types] > %launch_dir%\launch-layer.toml
echo launch = true >> %launch_dir%\launch-layer.toml

:: makes a cached launch layer
if not exist %launch_dir%\cached-launch-layer.toml (
    echo making cached launch layer %launch_dir%\cached-launch-layer
    mkdir %launch_dir%\cached-launch-layer
    echo Cached Dep Contents > %launch_dir%\cached-launch-layer\cached-dep
    mklink /j cached-deps %launch_dir%\cached-launch-layer
    echo [types] > %launch_dir%\cached-launch-layer.toml
    echo launch = true >> %launch_dir%\cached-launch-layer.toml
    echo cache = true >> %launch_dir%\cached-launch-layer.toml
) else (
    echo reusing cached launch layer %launch_dir%\cached-launch-layer
    mklink /j cached-deps %launch_dir%\cached-launch-layer
)

:: adds a process
(
echo [[processes]]
echo   type = "web"
echo   command = '.\run'
echo   args = ["8080"]
echo   default = true
echo.
echo [[processes]]
echo   type = "hello"
echo   command = "cmd"
echo   args = ["/c", "echo hello world"]
echo   direct = true
) > %launch_dir%\launch.toml

echo --- Done
