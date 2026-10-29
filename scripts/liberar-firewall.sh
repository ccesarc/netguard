#!/usr/bin/env bash
# Libera a porta do painel no firewall (UFW) só para a rede de casa (10.0.0.0/24).
# Pede a senha do sudo. Para desfazer:  sudo ufw delete allow from 10.0.0.0/24 to any port 8095 proto tcp
set -euo pipefail
REDE=10.0.0.0/24
PORTA=8095

sudo ufw allow from "$REDE" to any port "$PORTA" proto tcp comment 'NetGuard (so rede de casa)'
echo
sudo ufw status numbered | grep -E "Status|$PORTA" || true
