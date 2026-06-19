#!/bin/bash
# MeshVPN Быстрый старт сервера

set -e

echo "🚀 MeshVPN Server - Быстрый запуск"
echo "=================================="
echo ""

# Проверка Docker
if ! command -v docker &> /dev/null; then
    echo "❌ Docker не установлен!"
    echo "Установите Docker: sudo apt install -y docker.io docker-compose"
    exit 1
fi

echo "✓ Docker найден"

# Проверка docker-compose
if ! command -v docker-compose &> /dev/null; then
    echo "❌ Docker Compose не установлен!"
    echo "Установите: sudo apt install -y docker-compose"
    exit 1
fi

echo "✓ Docker Compose найден"
echo ""

# Получаем внешний IP
EXTERNAL_IP=$(curl -s ifconfig.me || curl -s icanhazip.com || echo "127.0.0.1")
echo "🌐 Определен внешний IP: $EXTERNAL_IP"
echo ""

# Генерируем случайный TURN secret
TURN_SECRET=$(openssl rand -base64 32 || date +%s | sha256sum | base64 | head -c 32)
echo "🔐 Сгенерирован TURN secret: $TURN_SECRET"
echo ""

# Создаем .env файл
cd deploy

echo "📝 Создаю конфигурацию..."
cat > .env <<EOF
# MeshVPN Server Configuration
# Auto-generated: $(date)

# Server settings
SERVER_DOMAIN=$EXTERNAL_IP
EXTERNAL_IP=$EXTERNAL_IP

# Database
DB_PATH=./data/meshvpn.db

# Ports
HTTP_PORT=8090
STUN_PORT=3479
TURN_PORT=5349

# TURN credentials
TURN_USER=meshvpn
TURN_SECRET=$TURN_SECRET

# Debug mode
DEBUG=true
EOF

echo "✓ Конфигурация создана: deploy/.env"
echo ""

# Создаем папку для данных
mkdir -p data
echo "✓ Создана папка для данных: deploy/data"
echo ""

# Настройка firewall (опционально)
read -p "🔥 Настроить firewall? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo "Настраиваю ufw..."
    sudo ufw allow 8090/tcp  # API
    sudo ufw allow 3479/udp  # STUN UDP
    sudo ufw allow 3479/tcp  # STUN TCP
    echo "✓ Firewall настроен"
fi
echo ""

# Запуск
echo "🚀 Запускаю сервер..."
docker-compose up -d

echo ""
echo "⏳ Ожидание запуска (5 секунд)..."
sleep 5

# Проверка
echo ""
echo "🧪 Проверка работоспособности..."
if curl -s http://localhost:8090/health > /dev/null; then
    echo "✅ Сервер успешно запущен!"
else
    echo "⚠️  Сервер запущен, но API пока не отвечает"
    echo "   Проверьте логи: docker-compose logs -f"
fi

echo ""
echo "=================================="
echo "✅ ГОТОВО!"
echo ""
echo "📋 Информация для подключения:"
echo "   API URL:  http://$EXTERNAL_IP:8090"
echo "   STUN:     $EXTERNAL_IP:3479"
echo ""
echo "📝 Полезные команды:"
echo "   Логи:     cd deploy && docker-compose logs -f"
echo "   Стоп:     cd deploy && docker-compose down"
echo "   Рестарт:  cd deploy && docker-compose restart"
echo ""
echo "🔐 TURN credentials (сохраните!):"
echo "   User:     meshvpn"
echo "   Secret:   $TURN_SECRET"
echo ""
