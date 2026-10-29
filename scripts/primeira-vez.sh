#!/usr/bin/env bash
# Prepara o NetGuard neste PC: configuração, usuário do painel e (se quiser) o serviço.
# Pode rodar de novo sem estragar nada.
#   ~/projects/netguard/scripts/primeira-vez.sh
set -euo pipefail
cd "$(dirname "$0")/.."
RAIZ=$PWD
CONF="$HOME/.config/netguard"
ENV="$CONF/netguard.env"

pergunta() { # pergunta "texto" -> 0 se sim
  local r
  read -r -p "$1 [s/N] " r
  [[ "$r" =~ ^[sS] ]]
}

echo "== NetGuard: primeira vez"

# 1. banco
if ! docker exec infra_pg pg_isready -q 2>/dev/null; then
  echo "-> ligando o postgres da infra"
  (cd "$HOME/projects/infra" && docker compose up -d postgres >/dev/null)
  for _ in $(seq 1 30); do docker exec infra_pg pg_isready -q 2>/dev/null && break; sleep 1; done
fi
echo "-> banco ok"

# 2. programa
if [[ ! -x bin/netguardd ]]; then
  echo "-> compilando (leva um minuto)"
  scripts/compilar.sh >/dev/null
fi
echo "-> programa ok: $(bin/netguardd versao)"

# 3. configuração
mkdir -p "$CONF" && chmod 700 "$CONF"
if [[ ! -f "$ENV" ]]; then
  cp deploy/netguard.env.exemplo "$ENV"
  echo "-> configuração criada em $ENV"
else
  echo "-> configuração já existe ($ENV)"
fi
sed -i "s#/home/USUARIO#$HOME#" "$ENV" # corrige arquivo copiado à mão do exemplo
chmod 600 "$ENV"
set -a; source "$ENV"; set +a

# 4. usuário do painel (códigos de saída: 2 = já existe, 3 = nome/senha inválidos)
echo
read -r -p "Nome de usuário do painel [$USER]: " nome
nome=${nome:-$USER}
acao=criar
while true; do
  codigo=0
  bin/netguardd usuario "$acao" "$nome" || codigo=$?
  case $codigo in
    0) break ;;
    2) if pergunta "Trocar a senha de $nome?"; then acao=senha; else break; fi ;;
    3) echo "Tente de novo (senha com 10 a 128 caracteres, digitada igual nas duas vezes)." ;;
    *) echo "Erro inesperado; nada foi mudado."; exit 1 ;;
  esac
done

# 5. serviço
echo
UNIT="$HOME/.config/systemd/user/netguardd.service"
if pergunta "Instalar como serviço (liga sozinho junto com o PC)?"; then
  install -Dm755 bin/netguardd "$HOME/.local/bin/netguardd"
  install -Dm644 deploy/netguardd.service "$UNIT"
  systemctl --user daemon-reload
  systemctl --user enable --now netguardd
  systemctl --user restart netguardd
  sleep 2
  systemctl --user --no-pager status netguardd | head -5
  echo
  echo "Pronto. Painel: http://10.0.0.209:8095 (ou http://127.0.0.1:8095)"
  echo "Logs: journalctl --user -u netguardd -f"
else
  echo
  echo "Ligando só agora (feche com Ctrl+C)."
  echo "Painel: http://10.0.0.209:8095 (ou http://127.0.0.1:8095)"
  exec "$RAIZ/bin/netguardd" servir
fi
