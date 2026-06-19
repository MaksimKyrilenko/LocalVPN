# MeshVPN - Аналог Radmin VPN

Самостоятельное решение для создания виртуальных частных сетей (VPN) с P2P-соединениями, аналогичное Radmin VPN.

## Возможности

- **P2P-соединения** — прямое соединение между устройствами без посредников
- **NAT Traversal** — автоматический обход NAT и фаерволов
- **Виртуальная LAN** — устройства видят друг друга как в локальной сети
- **Шифрование** — WireGuard протокол с ChaCha20-Poly1305
- **Кроссплатформенность** — Windows, Linux (macOS в планах)
- **Self-hosted** — полный контроль над инфраструктурой

## Архитектура

```
┌─────────────┐         ┌─────────────┐
│   Client A  │◄───────►│   Client B  │
│  (Windows)  │  P2P    │  (Windows)  │
│             │ WireGuard│            │
└──────┬──────┘         └──────┬──────┘
       │                       │
       │    ┌─────────────┐    │
       └───►│   Server    │◄───┘
            │  (VPS/RU)   │
            │ • Signaling │
            │ • STUN/TURN │
            │ • Coordination
            └─────────────┘
```

## Структура проекта

```
meshvpn/
├── server/           # Серверная часть (Go)
│   ├── cmd/          # Точка входа
│   ├── internal/     # Внутренние пакеты
│   └── migrations/   # Миграции БД
├── client/           # Клиентская часть (Go + Wails)
│   ├── cmd/          # CLI версия
│   └── internal/     # GUI и логика
├── deploy/           # Docker, configs
├── docs/             # Документация
└── examples/         # Примеры использования
```

## Быстрый старт

### 1. Деплой сервера на VPS

```bash
cd deploy
docker-compose up -d
```

### 2. Установка клиента (Windows)

Скачайте установщик из Releases или соберите из исходников.

### 3. Создание сети

1. Откройте клиент
2. Нажмите "Создать сеть"
3. Скопируйте Network ID
4. Поделитесь с друзьями

## Требования

**Сервер:**
- Linux VPS с публичным IP
- 1 CPU, 1GB RAM
- Docker + Docker Compose
- Открытые порты: 80, 443, 3478 (STUN), 5349 (TURNS), 10000-20000 (TURN relay)

**Клиент:**
- Windows 10/11 (x64)
- Права администратора (для создания TUN интерфейса)
- Visual C++ Redistributable

## Разработка

### Сервер

```bash
cd server
go mod download
go run cmd/server/main.go
```

### Клиент

```bash
cd client
wails dev
```

## Лицензия

MIT License
