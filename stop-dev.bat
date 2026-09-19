@echo off
chcp 65001 >nul
cd /d "%~dp0"
echo Konteynerlar to'xtatilmoqda...
docker compose -f docker-compose.yml -f deploy/docker-compose.dev.yml down
echo Tayyor.
pause
