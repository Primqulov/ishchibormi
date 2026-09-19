@echo off
chcp 65001 >nul
setlocal
cd /d "%~dp0"

echo ============================================
echo   Ishchi Bormi - LOKAL DEV ishga tushirish
echo ============================================
echo.

where docker >nul 2>&1
if errorlevel 1 (
  echo [XATO] Docker topilmadi.
  echo Docker Desktop o'rnating: https://www.docker.com/products/docker-desktop/
  echo O'rnatgandan keyin Docker Desktop'ni ishga tushirib, shu faylni qayta oching.
  pause
  exit /b 1
)

docker info >nul 2>&1
if errorlevel 1 (
  echo [XATO] Docker Desktop ishlamayapti.
  echo Docker Desktop'ni oching, "Engine running" yozuvini kuting, keyin shu faylni qayta oching.
  pause
  exit /b 1
)

if not exist ".env" (
  echo [XATO] .env fayli yo'q. .env.example dan nusxa oling.
  pause
  exit /b 1
)

echo [1/4] Konteynerlar quriladi va ishga tushiriladi (birinchi marta 3-10 daqiqa)...
docker compose -f docker-compose.yml -f deploy/docker-compose.dev.yml up --build -d
if errorlevel 1 (
  echo.
  echo [XATO] Ishga tushirishda xatolik. Loglar:
  docker compose logs --tail=80
  pause
  exit /b 1
)

echo.
echo [2/4] Backend tayyor bo'lishini kutamiz...
set /a tries=0
:waitloop
set /a tries+=1
curl -s -o nul -w "" http://localhost:8080/healthz >nul 2>&1
if not errorlevel 1 goto ready
if %tries% GEQ 60 (
  echo [OGOHLANTIRISH] Backend 60 soniyada javob bermadi. Loglarni ko'ring:
  docker compose logs --tail=60 backend
  goto after
)
timeout /t 2 /nobreak >nul
goto waitloop

:ready
echo Backend tayyor: http://localhost:8080/healthz

:after
echo.
echo [3/4] Demo ma'lumot (seed) kerakmi?
choice /c YN /n /m "Ha uchun Y, yo'q uchun N: "
if errorlevel 2 goto noseed
docker compose exec backend /app/seed
:noseed

echo.
echo [4/4] Holat:
docker compose ps

echo.
echo ============================================
echo   Frontend : http://localhost:3000
echo   API      : http://localhost:8080
echo   Mongo    : localhost:27018
echo   Bot      : konteynerda ishlayapti (tashqi port yo'q)
echo ============================================
echo.
echo Loglarni ko'rish     : docker compose logs -f
echo To'xtatish           : stop-dev.bat
echo.

start "" http://localhost:3000
pause
