# Сборка MeshVPN Client

## Windows GUI (Wails)

### Требования

- Windows 10/11 (x64)
- Go 1.22+
- Node.js 18+
- Wails CLI: `go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- Visual Studio Build Tools или MinGW-w64

### Установка зависимостей

1. **Установка Go:**
   ```powershell
   winget install GoLang.Go.1.22
   ```

2. **Установка Node.js:**
   ```powershell
   winget install OpenJS.NodeJS.LTS
   ```

3. **Установка Wails:**
   ```powershell
   go install github.com/wailsapp/wails/v2/cmd/wails@latest
   ```

4. **Установка GCC:**
   ```powershell
   winget install MinGW-w64
   # или
   winget install Microsoft.VisualStudio.2022.BuildTools
   ```

### Сборка

```powershell
cd client

# Установка зависимостей
wails deps

# Сборка
wails build

# Для production
wails build -platform windows/amd64 -ldflags "-w -s" -trimpath
```

Результат: `client/build/bin/MeshVPN.exe`

### Сборка установщика

```powershell
# Установка Inno Setup
winget install InnoSetup

# Создание скрипта установки
iscc installer.iss
```

## Windows CLI

```powershell
cd client/cmd/cli

# Сборка
go build -o meshvpn-cli.exe main.go

# Production build
go build -ldflags "-w -s" -trimpath -o meshvpn-cli.exe main.go
```

## Linux

### CLI версия

```bash
cd client/cmd/cli

# Обычная сборка
go build -o meshvpn-cli main.go

# Установка
sudo cp meshvpn-cli /usr/local/bin/
sudo chmod +x /usr/local/bin/meshvpn-cli
```

### GUI версия

```bash
cd client

# Требуется CGO и GTK/webkit
go build -tags desktop,wv2runtime
go build -o meshvpn-gui main.go
```

## Cross-compilation

### Linux -> Windows

```bash
# Установка mingw
sudo apt install mingw-w64

# Сборка CLI
GOOS=windows GOARCH=amd64 CGO_ENABLED=1 \
  CC=x86_64-w64-mingw32-gcc \
  go build -o meshvpn-cli.exe ./cmd/cli
```

### Windows -> Linux

```powershell
# Используем Docker или WSL
wsl -e bash -c "cd client/cmd/cli && go build -o meshvpn-cli main.go"
```

## Разработка

### Запуск в dev режиме

```powershell
cd client
wails dev
```

Это запустит:
- Vite dev server для frontend
- Wails с hot-reload

### Структура frontend

```
frontend/
├── src/
│   ├── index.html    # Главный HTML
│   ├── style.css     # Стили
│   └── app.js        # Логика
└── package.json      # Конфигурация
```

### Доступные команды Wails

```powershell
wails dev          # Dev режим с hot-reload
wails build        # Production сборка
wails init         # Создать новый проект
wails doctor       # Проверка окружения
wails update       # Обновление Wails
```

## Возможные проблемы

### "gcc not found"

```powershell
# Проверка
where gcc

# Если не найдено, добавьте в PATH:
# C:\ProgramData\mingw64\mingw64\bin
```

### "webview2 not found"

```powershell
# Установка WebView2 Runtime
winget install Microsoft.EdgeWebView2Runtime
```

### "node_modules not found"

```powershell
cd client/frontend
npm install
```

## Создание релиза

### GitHub Actions

Создайте `.github/workflows/release.yml`:

```yaml
name: Release

on:
  push:
    tags:
      - 'v*'

jobs:
  build-windows:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v3
      
      - uses: actions/setup-go@v4
        with:
          go-version: '1.22'
      
      - uses: actions/setup-node@v3
        with:
          node-version: '18'
      
      - name: Install Wails
        run: go install github.com/wailsapp/wails/v2/cmd/wails@latest
      
      - name: Build
        run: |
          cd client
          wails build -platform windows/amd64
      
      - name: Upload
        uses: actions/upload-artifact@v3
        with:
          name: MeshVPN-Windows
          path: client/build/bin/*.exe
```

### Ручной релиз

```powershell
# Сборка всех версий
$version = "1.0.0"

# Windows GUI
wails build -o "MeshVPN-$version.exe"

# Windows CLI
cd cmd/cli
go build -o "meshvpn-cli-$version.exe"

# Linux
cd ../..
# (сборка в WSL или Docker)

# Создание архивов
Compress-Archive -Path "MeshVPN-$version.exe" -DestinationPath "MeshVPN-Windows-$version.zip"
```

## Подпись исполняемых файлов (рекомендуется)

```powershell
# Требуется сертификат Code Signing
signtool.exe sign /f certificate.pfx /p password MeshVPN.exe
```
