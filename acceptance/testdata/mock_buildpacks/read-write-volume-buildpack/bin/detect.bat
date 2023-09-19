@echo off

set TEST_FILE_PATH=%DETECT_TEST_FILE_PATH%

echo --- Detect: Read/Write Volume Buildpack

echo some-content> %TEST_FILE_PATH%
if exist %TEST_FILE_PATH% (
    echo Detect: Writing file '%TEST_FILE_PATH%': written
) else (
    echo Detect: Writing file '%TEST_FILE_PATH%': failed
)

set /p content=<%TEST_FILE_PATH%
echo Detect: Reading file '%TEST_FILE_PATH%': %content%

echo --- Done
