@echo off

echo --- Detect: Volume Buildpack

set /p content=<%TEST_FILE_PATH%
echo Detect: Reading file '%TEST_FILE_PATH%': %content%

echo --- Done
