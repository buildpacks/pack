@echo off
echo --- Build: Simple Asset Layers Buildpack

set launch_dir=%1

:: makes a launch layer
echo making launch layer %launch_dir%\launch-layer
mkdir %launch_dir%\launch-layer
echo Launch Dep Contents > "%launch_dir%\launch-layer\launch-dep
mklink /j launch-deps %launch_dir%\launch-layer
echo launch = true > %launch_dir%\launch-layer.toml

:: makes a cached launch layer
if not exist %launch_dir%\cached-launch-layer.toml (
    echo making cached launch layer %launch_dir%\cached-launch-layer
    mkdir %launch_dir%\cached-launch-layer
    echo Cached Dep Contents > %launch_dir%\cached-launch-layer\cached-dep
    mklink /j cached-deps %launch_dir%\cached-launch-layer
    echo launch = true > %launch_dir%\cached-launch-layer.toml
    echo cache = true >> %launch_dir%\cached-launch-layer.toml
) else (
    echo reusing cached launch layer %launch_dir%\cached-launch-layer
    mklink /j cached-deps %launch_dir%\cached-launch-layer
)

if exist "/cnb/assets/797b3f6bf2b2c10a8299d51dfdbcfed329d3c133fdc7e695beddbe8f70b49da9" (
  echo "Asset A exists!"
  type "/cnb/assets/797b3f6bf2b2c10a8299d51dfdbcfed329d3c133fdc7e695beddbe8f70b49da9"
)

if exist "/cnb/assets/61eea2ec4053ca25b9bd5d7bebaba48ee5398569aa1da5bc3541cbab1d09b86b" (
  echo "Asset B exists!"
  type "/cnb/assets/61eea2ec4053ca25b9bd5d7bebaba48ee5398569aa1da5bc3541cbab1d09b86b"
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
