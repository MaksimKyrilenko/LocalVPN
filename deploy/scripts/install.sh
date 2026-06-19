#!/bin/bash
# MeshVPN Server Quick Install Script
# Usage: curl -fsSL https://your-domain.ru/install.sh | bash

set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log() {
    echo -e "${GREEN}[MeshVPN]${NC} $1"
}

warn() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

error() {
    echo -e "${RED}[ERROR]${NC} $1"
    exit 1
}

# Check if running as root
if [[ $EUID -ne 0 ]]; then
   error "This script must be run as root. Use: sudo bash install.sh"
fi

# Get server IP
SERVER_IP=$(curl -s ifconfig.me || curl -s icanhazip.com || hostname -I | awk '{print $1}')
log "Detected server IP: $SERVER_IP"

# Check OS
if ! command -v apt-get &> /dev/null; then
    error "This installer supports only Debian/Ubuntu systems"
fi

# Install dependencies
log "Installing dependencies..."
apt-get update
apt-get install -y curl wget git docker.io docker-compose

# Start Docker
systemctl enable docker
systemctl start docker

# Create installation directory
INSTALL_DIR="/opt/meshvpn"
mkdir -p $INSTALL_DIR
cd $INSTALL_DIR

# Download docker-compose.yml
log "Downloading configuration..."
curl -fsSL -o docker-compose.yml https://raw.githubusercontent.com/yourusername/meshvpn/main/deploy/docker-compose.yml
curl -fsSL -o Dockerfile https://raw.githubusercontent.com/yourusername/meshvpn/main/deploy/Dockerfile
curl -fsSL -o Caddyfile https://raw.githubusercontent.com/yourusername/meshvpn/main/deploy/Caddyfile
curl -fsSL -o .env.example https://raw.githubusercontent.com/yourusername/meshvpn/main/deploy/.env.example

# Create .env file
log "Creating configuration..."
cat > .env <<EOF
SERVER_DOMAIN=$SERVER_IP
EXTERNAL_IP=$SERVER_IP
DB_PATH=./data/meshvpn.db
HTTP_PORT=8080
STUN_PORT=3478
TURN_PORT=5349
TURN_USER=meshvpn
TURN_SECRET=$(openssl rand -base64 32)
DEBUG=false
EOF

# Create data directory
mkdir -p data

# Ask user for domain
read -p "Enter your domain (or press Enter to use IP: $SERVER_IP): " DOMAIN
if [ ! -z "$DOMAIN" ]; then
    sed -i "s/SERVER_DOMAIN=.*/SERVER_DOMAIN=$DOMAIN/" .env
fi

# Start services
log "Starting MeshVPN Server..."
docker-compose up -d

# Wait for startup
sleep 5

# Check if running
if docker-compose ps | grep -q "Up"; then
    log "MeshVPN Server is running!"
    echo
    echo "═══════════════════════════════════════════════════"
    echo "  Installation Complete!"
    echo "═══════════════════════════════════════════════════"
    echo
    echo "Server URL: http://$SERVER_IP:8080"
    if [ ! -z "$DOMAIN" ]; then
        echo "Domain: https://$DOMAIN"
    fi
    echo
    echo "API Endpoints:"
    echo "  Health: http://$SERVER_IP:8080/health"
    echo "  Info:   http://$SERVER_IP:8080/info"
    echo
    echo "STUN Server: $SERVER_IP:3478"
    echo
    echo "Next steps:"
    echo "  1. Open ports in firewall:"
    echo "     ufw allow 8080/tcp"
    echo "     ufw allow 3478/udp"
    echo "     ufw allow 3478/tcp"
    echo
    echo "  2. Download client and connect to:"
    echo "     http://$SERVER_IP:8080"
    echo
    echo "  3. For HTTPS setup, see:"
    echo "     https://github.com/yourusername/meshvpn/blob/main/docs/DEPLOY.md"
    echo
    echo "  4. View logs:"
    echo "     cd $INSTALL_DIR && docker-compose logs -f"
    echo
    echo "═══════════════════════════════════════════════════"
else
    error "Failed to start MeshVPN Server. Check logs: docker-compose logs"
fi

# Create management script
cat > /usr/local/bin/meshvpn <<'EOF'
#!/bin/bash
cd /opt/meshvpn

case "$1" in
    start)
        docker-compose up -d
        echo "MeshVPN Server started"
        ;;
    stop)
        docker-compose down
        echo "MeshVPN Server stopped"
        ;;
    restart)
        docker-compose restart
        echo "MeshVPN Server restarted"
        ;;
    status)
        docker-compose ps
        ;;
    logs)
        docker-compose logs -f
        ;;
    update)
        docker-compose pull
        docker-compose up -d
        echo "MeshVPN Server updated"
        ;;
    *)
        echo "Usage: meshvpn {start|stop|restart|status|logs|update}"
        exit 1
        ;;
esac
EOF

chmod +x /usr/local/bin/meshvpn

log "Installation complete! Use 'meshvpn' command to manage the server."
