#!/usr/bin/env bash
# Permite ao usuário atual BUSCAR redes Wi-Fi sem estar logado na tela do PC
# (sessão remota e o serviço do NetGuard). Só a busca: não deixa conectar,
# desconectar nem mudar nenhuma configuração de rede. Pede a senha do sudo.
#
# Para desfazer:  sudo rm /etc/polkit-1/rules.d/50-netguard-busca-wifi.rules
set -euo pipefail

ARQ=/etc/polkit-1/rules.d/50-netguard-busca-wifi.rules
USUARIO=$(id -un)

REGRA="// NetGuard: deixa o usuário $USUARIO buscar redes Wi-Fi (só a busca), mesmo
// sem estar logado na tela. Criado por ~/projects/netguard/scripts/permitir-busca-wifi.sh
// Para desfazer: sudo rm $ARQ
polkit.addRule(function(action, subject) {
    if (action.id == \"org.freedesktop.NetworkManager.wifi.scan\" &&
        subject.user == \"$USUARIO\") {
        return polkit.Result.YES;
    }
});"

echo "Vai criar $ARQ com:"
echo "----------------------------------------"
echo "$REGRA"
echo "----------------------------------------"
read -rp "Continuar? [s/N] " r
[[ "$r" =~ ^[sS] ]] || { echo "Cancelado."; exit 0; }

echo "$REGRA" | sudo tee "$ARQ" >/dev/null
sudo chmod 644 "$ARQ"
echo "Regra criada. Testando a busca..."
sleep 2
exec "$(dirname "$0")/testar-busca-wifi.sh"
