@echo off
echo Building TUNTray...

REM Build for Windows GUI (no console window)
go build -ldflags "-H windowsgui" -o build\TUNTray\TUNTray.exe .

echo Build completed!
echo Output: build\TUNTray\TUNTray.exe
pause
