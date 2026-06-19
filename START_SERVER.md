# 🚀 Запуск MeshVPN Сервера

## Вариант 1: Docker (Рекомендуется) ⭐

### Шаг 1: Настройка конфига
```bash
cd deploy
nano .env
```

**Измените:**
- `EXTERNAL_IP` — замените на IP вашего VPS
- `TURN_SECRET` — придумайте случайный пароль

### Шаг 2: Запуск
```bash
# Базовый запуск (только сервер + STUN)
docker-compose up -d

# С логами для проверки
docker-compose up
```

### Шаг 3: Проверка
```bash
# Проверка что сервер запустился
curl http://localhost:8090/health

# Логи
docker-compose logs -f meshvpn-server
```

**Сервер доступен:**
- API: `http://ВАШ_IP:8090`
- STUN: `ВАШ_IP:3479/udp`

---

## Вариант 2: Без Docker (Вручную)

### Шаг 1: Установка Go
```bash
wget https://go.dev/dl/go1.22.0.linux-amd64.tar.gz
sudo tar -C /usr/local -xzf go1.22.0.linux-amd64.tar.gz
echo 'export PATH=$PATH:/usr/local/go/bin' >> ~/.bashrc
source ~/.bashrc
```

### Шаг 2: Сборка
```bash
cd server
go mod download
go build -o meshvpn-server cmd/server/main.go
```

### Шаг 3: Запуск
```bash
# Создать папку для БД
mkdir -p data

# Запустить
./meshvpn-server -http=:8080 -db=./data/meshvpn.db -stun=:3478 -debug
```

---

## ⚙️ Настройка Firewall (Важно!)

```bash
# Ubuntu/Debian
sudo ufw allow 8080/tcp   # API
sudo ufw allow 3478/udp   # STUN
sudo ufw allow 3478/tcp   # STUN TCP
sudo ufw enable

# CentOS/RHEL
sudo firewall-cmd --permanent --add-port=8080/tcp
sudo firewall-cmd --permanent --add-port=3478/udp
sudo firewall-cmd --permanent --add-port=3478/tcp
sudo firewall-cmd --reload
```

---

## 🧪 Тестирование

### Проверка API
```bash
# Должно вернуть: {"status":"healthy",...}
curl http://ВАШ_IP:8080/health
```

### Проверка STUN (с другой машины)
```bash
# Установить stun-client
sudo apt install stuntman-client

# Проверить
stun ВАШ_IP -p 3478
```

---

## 📝 Полезные команды

```bash
# Остановить
docker-compose down

# Рестарт
docker-compose restart

# Логи
docker-compose logs -f

# Статус
docker-compose ps

# Очистка данных (ОСТОРОЖНО!)
rm -rf deploy/data/*
```

---

## 🔧 Решение проблем

### Порт уже занят
```bash
# Проверить что занимает порт
sudo netstat -tlnp | grep 8080

# Убить процесс
sudo kill -9 PID
```

### Docker не запускается
```bash
# Проверить статус
sudo systemctl status docker

# Запустить Docker
sudo systemctl start docker
```

### Сервер не отвечает снаружи
1. Проверьте firewall: `sudo ufw status`
2. Проверьте что порты открыты в панели VPS провайдера
3. Проверьте логи: `docker-compose logs`

---

## ✅ Готово!

После запуска сервер готов принимать клиентов:
- URL для клиента: `http://ВАШ_IP:8090` (или :8080 без Docker)
- STUN сервер: `ВАШ_IP:3479` (или :3478 без Docker)

**Следующий шаг:** Настройте клиент и подключитесь к серверу.
