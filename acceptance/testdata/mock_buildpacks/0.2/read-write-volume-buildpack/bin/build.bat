@echo off

set TEST_FILE_PATH=%BUILD_TEST_FILE_PATH%

echo --- Build: Read/Write Volume Buildpack

echo some-content> %TEST_FILE_PATH%
if exist %TEST_FILE_PATH% (
    echo Build: Writing file '%TEST_FILE_PATH%': written
) else (
    echo Build: Writing file '%TEST_FILE_PATH%': failed
)

set /p content=<%TEST_FILE_PATH%
echo Build: Reading file '%TEST_FILE_PATH%': %content%

echo --- Done
