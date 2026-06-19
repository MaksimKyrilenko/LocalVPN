# Инструкция по деплою MeshVPN Server

## Содержание
- [Требования](#требования)
- [Выбор VPS провайдера](#выбор-vps-провайдера)
- [Быстрый старт](#быстрый-старт)
- [Ручная установка](#ручная-установка)
- [Настройка HTTPS](#настройка-https)
- [Настройка TURN сервера](#настройка-turn-сервера)
- [Мониторинг](#мониторинг)
- [Решение проблем](#решение-проблем)

---

## Требования

### Минимальные
- **CPU:** 1 ядро
- **RAM:** 512 MB (1 GB рекомендуется)
- **Диск:** 5 GB SSD
- **Сеть:** 100 Mbps, публичный IP
- **OS:** Ubuntu 20.04/22.04 LTS, Debian 11/12

### Рекомендуемые
- **CPU:** 2 ядра
- **RAM:** 2 GB
- **Диск:** 10 GB SSD
- **Сеть:** 1 Gbps, публичный IP
- **Трафик:** Безлимитный (или минимум 1 TB/мес)

### Необходимые порты
| Порт | Протокол | Назначение |
|------|----------|------------|
| 80 | TCP | HTTP (для Let's Encrypt) |
| 443 | TCP | HTTPS |
| 3478 | UDP/TCP | STUN |
| 5349 | UDP/TCP | TURN TLS |
| 10000-20000 | UDP | TURN Relay |
| 8080 | TCP | API (внутренний) |

---

## Выбор VPS провайдера в России

### Рекомендуемые

| Провайдер | Цена/мес | Плюсы | Минусы |
|-----------|----------|-------|--------|
| **Selectel** | 400-800₽ | Стабильность, поддержка | Нет бесплатного трафика |
| **Beget** | 300-600₽ | Простой интерфейс | Меньше датацентров |
| **Timeweb** | 200-500₽ | Дешево | Меньше функций |
| **Yandex Cloud** | 500-1000₽ | Надёжность | Сложная ценовая модель |

### Выбор датацентра
- **Москва** — лучшая связность по РФ
- **Санкт-Петербург** — хорошая связность с Европой
- **Казань/Екатеринбург** — для географического распределения

---

## Быстрый старт

### 1. Подготовка сервера

```bash
# Обновление системы
sudo apt update && sudo apt upgrade -y

# Установка Docker
sudo apt install -y docker.io docker-compose

# Запуск Docker
sudo systemctl enable docker
sudo systemctl start docker

# Добавление пользователя в docker group
sudo usermod -aG docker $USER
# Перелогиньтесь после этого
```

### 2. Клонирование и деплой

```bash
# Клонируйте репозиторий
git clone https://github.com/yourusername/meshvpn.git
cd meshvpn/deploy

# Копируем пример конфигурации
cp .env.example .env

# Редактируем .env
nano .env
```

```bash
# .env
SERVER_DOMAIN=your-domain.ru
EXTERNAL_IP=YOUR_SERVER_IP
TURN_SECRET=change_this_to_random_string
```

### 3. Запуск

```bash
# Базовый запуск
docker-compose up -d

# С HTTPS (Caddy)
docker-compose --profile with-https up -d

# С TURN сервером
docker-compose --profile with-turn up -d

# Полная версия
docker-compose --profile with-https --profile with-turn up -d
```

### 4. Проверка

```bash
# Проверка сервера
curl http://your-server-ip:8080/health

# Должно вернуть: {"status":"healthy","time":...}

# Проверка STUN
nc -u -v your-server-ip 3478
```

---

## Ручная установка

Если Docker не подходит:

### 1. Установка зависимостей

```bash
sudo apt update
sudo apt install -y git golang-1.22 sqlite3

# Установка Go 1.22
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
export PATH=$PATH:/usr/local/go/bin
```

### 2. Сборка сервера

```bash
cd server
go mod download
go build -o meshvpn-server cmd/server/main.go
```

### 3. Создание systemd сервиса

```bash
sudo nano /etc/systemd/system/meshvpn.service
```

```ini
[Unit]
Description=MeshVPN Server
After=network.target

[Service]
Type=simple
User=meshvpn
Group=meshvpn
WorkingDirectory=/opt/meshvpn
ExecStart=/opt/meshvpn/meshvpn-server \
    -http=:8080 \
    -db=/opt/meshvpn/data/meshvpn.db \
    -stun=:3478
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
```

```bash
# Создаем пользователя
sudo useradd -r -s /bin/false meshvpn
sudo mkdir -p /opt/meshvpn/data
sudo chown -R meshvpn:meshvpn /opt/meshvpn

# Копируем бинарник
sudo cp meshvpn-server /opt/meshvpn/
sudo chmod +x /opt/meshvpn/meshvpn-server

# Запуск
sudo systemctl daemon-reload
sudo systemctl enable meshvpn
sudo systemctl start meshvpn
sudo systemctl status meshvpn
```

---

## Настройка HTTPS

### Вариант 1: Caddy (рекомендуется)

Уже включен в docker-compose. Просто укажите домен:

```bash
# Caddyfile
your-domain.ru {
    reverse_proxy meshvpn-server:8080
}
```

### Вариант 2: Nginx + Let's Encrypt

```bash
sudo apt install -y nginx certbot python3-certbot-nginx

# Получаем сертификат
sudo certbot --nginx -d your-domain.ru

# Nginx config
sudo nano /etc/nginx/sites-available/meshvpn
```

```nginx
server {
    listen 443 ssl http2;
    server_name your-domain.ru;

    ssl_certificate /etc/letsencrypt/live/your-domain.ru/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/your-domain.ru/privkey.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    location /ws {
        proxy_pass http://localhost:8080/ws;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
    }
}

server {
    listen 80;
    server_name your-domain.ru;
    return 301 https://$server_name$request_uri;
}
```

---

## Настройка TURN сервера

TURN нужен когда P2P невозможен (симметричный NAT).

### Вариант 1: Встроенный (простой)

```bash
# Запуск с TURN профилем
docker-compose --profile with-turn up -d
```

### Вариант 2: Coturn отдельно

```bash
sudo apt install -y coturn

sudo nano /etc/turnserver.conf
```

```conf
listening-port=3478
tls-listening-port=5349
listening-ip=YOUR_SERVER_IP
relay-ip=YOUR_SERVER_IP
external-ip=YOUR_SERVER_IP
min-port=10000
max-port=20000

# Аутентификация
user=meshvpn:YOUR_SECRET_KEY
realm=your-domain.ru

# Безопасность
no-multicast-peers
no-loopback-peers

# Логирование
verbose
log-file=/var/log/turnserver.log
```

```bash
sudo systemctl restart coturn
```

---

## Мониторинг

### Логи

```bash
# Docker
sudo docker-compose logs -f meshvpn-server

# Systemd
sudo journalctl -u meshvpn -f
```

### Метрики

```bash
# Количество подключенных пиров
curl http://localhost:8080/api/v1/networks/NETWORK_ID/peers

# Проверка здоровья
curl http://localhost:8080/health
```

### Prometheus + Grafana (опционально)

Добавьте в docker-compose:

```yaml
  prometheus:
    image: prom/prometheus
    volumes:
      - ./prometheus.yml:/etc/prometheus/prometheus.yml
    ports:
      - "9090:9090"

  grafana:
    image: grafana/grafana
    ports:
      - "3000:3000"
```

---

## Решение проблем

### Сервер не запускается

```bash
# Проверка портов
sudo netstat -tlnp | grep 8080
sudo netstat -uln | grep 3478

# Логи
sudo docker-compose logs meshvpn-server
# или
sudo journalctl -u meshvpn -n 100
```

### Клиент не подключается

```bash
# Проверка с сервера
curl http://localhost:8080/health

# Проверка STUN
stun-client your-server-ip:3478

# Проверка извне
curl http://your-server-ip:8080/health
```

### Проблемы с NAT Traversal

1. Убедитесь что UDP порты открыты:
```bash
sudo iptables -L -n -v | grep 3478
```

2. Проверьте TURN сервер:
```bash
# На клиенте
turnutils_uclient -u meshvpn -w YOUR_SECRET your-server-ip
```

### Высокая задержка

- Проверьте геолокацию сервера
- Включите TURN relay если P2P не работает
- Проверьте скорость сети: `iperf3`

---

## Обновление

```bash
cd meshvpn
git pull

# Docker
docker-compose down
docker-compose up -d

# Systemd
sudo systemctl stop meshvpn
sudo cp server/meshvpn-server /opt/meshvpn/
sudo systemctl start meshvpn
```

---

## Безопасность

### Базовые меры

1. **Фаервол:**
```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing
sudo ufw allow 22/tcp
sudo ufw allow 80/tcp
sudo ufw allow 443/tcp
sudo ufw allow 3478/udp
sudo ufw allow 3478/tcp
sudo ufw enable
```

2. **Fail2ban:**
```bash
sudo apt install -y fail2ban
sudo systemctl enable fail2ban
```

3. **Обновления:**
```bash
sudo apt install -y unattended-upgrades
```

### Продвинутые меры

- Используйте WireGuard для доступа к серверу
- Отключите парольный SSH, используйте ключи
- Включите 2FA если провайдер поддерживает

---

## Поддержка

При проблемах:
1. Проверьте логи
2. Проверьте открытые порты
3. Проверьте firewall
4. Создайте Issue с логами

---

**Готово к использованию!** 🚀
