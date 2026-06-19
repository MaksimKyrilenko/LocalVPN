#!/bin/bash
# Setup firewall for MeshVPN Server

set -e

echo "Setting up firewall for MeshVPN..."

# Check if ufw is installed
if ! command -v ufw &> /dev/null; then
    echo "Installing ufw..."
    apt-get update
    apt-get install -y ufw
fi

# Reset ufw (optional - be careful if using SSH!)
# ufw --force reset

# Default policies
ufw default deny incoming
ufw default allow outgoing

# SSH (don't lock yourself out!)
echo "Allowing SSH..."
ufw allow 22/tcp

# HTTP/HTTPS
ufw allow 80/tcp
ufw allow 443/tcp

# MeshVPN API
ufw allow 8080/tcp

# STUN
ufw allow 3478/tcp
ufw allow 3478/udp

# TURN TLS
ufw allow 5349/tcp
ufw allow 5349/udp

# TURN Relay ports
ufw allow 10000:20000/udp

# Enable firewall
echo "Enabling firewall..."
ufw --force enable

echo ""
echo "Firewall configured!"
echo ""
ufw status verbose
