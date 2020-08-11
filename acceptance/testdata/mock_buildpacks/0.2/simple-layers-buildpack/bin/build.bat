@echo off
echo --- Build: Simple Layers Buildpack

set launch_dir=%1

:: makes a launch layer
echo making launch layer
mkdir %launch_dir%\launch-layer
echo Launch Dep Contents > "%launch_dir%\launch-layer\launch-dep
mklink launch-dep %launch_dir%\launch-layer\launch-dep
echo launch = true > %launch_dir%\launch-layer.toml

:: makes a cached launch layer
if not exist %launch_dir%\cached-launch-layer.toml (
    echo making cached launch layer
    mkdir %launch_dir%\cached-launch-layer
    echo Cached Dep Contents > %launch_dir%\cached-launch-layer\cached-dep
    mklink cached-dep %launch_dir%\cached-launch-layer\cached-dep
    echo launch = true > %launch_dir%\cached-launch-layer.toml
    echo cache = true >> %launch_dir%\cached-launch-layer.toml
) else (
  echo reusing cached launch layer
  mklink cached-dep %launch_dir%\cached-launch-layer\cached-dep
)

:: adds a process
(
echo [[processes]]
echo   type = "web"
echo   command = '.\run'
echo   args = ["8080"]
echo.
echo [[processes]]
echo   type = "hello"
echo   command = "cmd"
echo   args = ["/c", "echo hello world"]
echo   direct = true
) > %launch_dir%\launch.toml

echo --- Done
