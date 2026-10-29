#!/usr/bin/env bash
# Testa se o Wi-Fi deste PC enxerga as redes vizinhas (só leitura; não muda nada).
# Rodar no Konsole, logado na tela: a busca só é permitida para quem está na tela.
set -uo pipefail
echo "Buscando redes Wi-Fi (leva uns 5 segundos)..."
if ! nmcli dev wifi rescan; then
  echo "A busca foi recusada pelo sistema."
  exit 1
fi
sleep 6
echo
nmcli -f SSID,CHAN,SIGNAL,BARS,SECURITY dev wifi list --rescan no
echo
echo "Total de redes vistas: $(nmcli -t -f SSID dev wifi list --rescan no | wc -l)"
