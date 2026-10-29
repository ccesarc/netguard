#!/usr/bin/env bash
# Menu do NetGuard: escolha pelo número.
#
#   ./scripts/menu.sh        (também pelo `painel` → NetGuard)
#
# Ligar/desligar o serviço, estado, logs, atualizar depois de mudar o código, testes, usuários e
# firewall. O serviço é do usuário (systemctl --user), sem root. Nada aqui imprime senha.
set -uo pipefail

RAIZ="$(cd "$(dirname "$0")/.." && pwd)"
cd "$RAIZ"
SERVICO=netguardd
UNIT="$HOME/.config/systemd/user/$SERVICO.service"
BIN="$HOME/.local/bin/netguardd"
ENV="$HOME/.config/netguard/netguard.env"
PORTA=8095

n=$'\033[1m'; v=$'\033[32m'; a=$'\033[33m'; r=$'\033[31m'; c=$'\033[36m'; d=$'\033[2m'; f=$'\033[0m'

titulo() { printf '\n%s▸ %s%s\n' "$n" "$1" "$f"; }
ok() { printf '%s✓ %s%s\n' "$v" "$1" "$f"; }
aviso() { printf '%s! %s%s\n' "$a" "$1" "$f"; }
erro() { printf '%s✗ %s%s\n' "$r" "$1" "$f"; }
pausa() { printf '\n%sEnter para voltar ao menu...%s' "$c" "$f"; read -r _ || exit 0; }

# sim "pergunta" [padrao s|n] → 0 se sim. Fim da entrada = não.
sim() {
  local padrao=${2:-n} resp dica='[s/N]'
  [[ $padrao == s ]] && dica='[S/n]'
  read -rp "$1 $dica " resp || return 1
  resp=${resp:-$padrao}
  [[ ${resp,,} == s* ]]
}

# escolher "pergunta" opções... → ecoa a escolhida (fim da entrada = falha)
escolher() {
  local pergunta=$1 i=1 o resp
  shift
  printf '%s\n' "$pergunta" >&2
  for o in "$@"; do
    printf '  %s%d)%s %s\n' "$c" "$i" "$f" "$o" >&2
    ((i++))
  done
  while read -rp "> " resp; do
    if [[ $resp =~ ^[0-9]+$ ]] && ((resp >= 1 && resp <= $#)); then
      echo "${!resp}"
      return 0
    fi
    printf 'Digite um número de 1 a %d.\n' $# >&2
  done
  return 1
}

ip_casa() { ip -4 -br addr show wlan0 2>/dev/null | awk '{print $3}' | cut -d/ -f1; }
instalado() { [[ -f $UNIT && -x $BIN ]]; }
rodando() { systemctl --user is-active -q "$SERVICO"; }
# Sem "linger", o systemd derruba os serviços do usuário (NetGuard e o Docker do
# postgres) quando a última sessão fecha — sair do acesso remoto desligava tudo.
sem_sessao() { [[ $(loginctl show-user "$USER" -p Linger --value 2>/dev/null) == yes ]]; }
carregar_env() { [[ -f $ENV ]] && { set -a; source "$ENV"; set +a; }; }

banco_no_ar() {
  docker exec infra_pg pg_isready -q 2>/dev/null && return 0
  aviso "Postgres da infra desligado: ligando."
  (cd "$HOME/projects/infra" && docker compose up -d postgres >/dev/null 2>&1)
  for _ in $(seq 1 30); do docker exec infra_pg pg_isready -q 2>/dev/null && return 0; sleep 1; done
  erro "Postgres não subiu. Veja: cd ~/projects/infra && docker compose logs postgres"
  return 1
}

compilar() {
  titulo "Compilando (frontend + backend)"
  scripts/compilar.sh 2>&1 | tail -3 || { erro "Compilação falhou."; return 1; }
}

instalar_binario() {
  install -Dm755 bin/netguardd "$BIN"
  install -Dm644 deploy/netguardd.service "$UNIT"
  systemctl --user daemon-reload
}

estado() {
  titulo "Estado"
  if ! instalado; then
    aviso "Serviço não instalado (opção 2 instala e liga)."
  elif rodando; then
    local pid mem
    pid=$(systemctl --user show -p MainPID --value "$SERVICO")
    mem=$(awk '/VmRSS/ {printf "%.0f MB", $2/1024}' /proc/"$pid"/status 2>/dev/null)
    ok "Serviço ligado (memória ${mem:-?}, $(systemctl --user is-enabled "$SERVICO" 2>/dev/null | sed 's/enabled/liga com o PC/;s/disabled/não liga sozinho/'))"
  else
    erro "Serviço desligado."
  fi
  sem_sessao || aviso "Para quando você sai da sessão (acesso remoto ou login). Opção 2 resolve."
  local saude
  saude=$(curl -s -m 3 "http://127.0.0.1:$PORTA/api/v1/saude")
  if [[ -n $saude ]]; then
    printf '%s\n' "$saude" | python3 -c '
import json, sys
s = json.load(sys.stdin)
print(f"Saúde: {s.get(\"status\")} · banco {s.get(\"banco\")} · versão {s.get(\"versao\")}")
print(f"Última medição: {s.get(\"ultima_medicao\") or \"nenhuma ainda\"}")'
  fi
  printf 'Painel: %shttp://%s:%s%s  %s(neste PC: http://127.0.0.1:%s)%s\n' "$n" "$(ip_casa)" "$PORTA" "$f" "$d" "$PORTA" "$f"
  printf '%sCódigo:%s %s\n' "$d" "$f" "$(git log --oneline -1 2>/dev/null || echo 'sem commit')"
  [[ -x $BIN && bin/netguardd -nt $BIN ]] && aviso "bin/netguardd é mais novo que o instalado: use a opção 5 (atualizar)."
}

ligar() {
  titulo "Ligar"
  banco_no_ar || return
  if ! instalado; then
    [[ -x bin/netguardd ]] || compilar || return
    instalar_binario
    ok "Serviço instalado."
  fi
  if sim "Ligar sozinho junto com o PC?" s; then
    systemctl --user enable --now "$SERVICO"
  else
    systemctl --user start "$SERVICO"
  fi
  if ! sem_sessao && sim "Continuar ligado quando você sair da sessão (acesso remoto)?" s; then
    loginctl enable-linger "$USER" && ok "Fica ligado mesmo sem ninguém logado."
  fi
  sleep 2
  rodando && ok "Ligado: http://$(ip_casa):$PORTA" || { erro "Não ligou. Veja o log (opção 4)."; }
}

desligar() {
  titulo "Desligar"
  rodando || { aviso "Já está desligado."; return; }
  systemctl --user stop "$SERVICO" && ok "Desligado."
  if [[ $(systemctl --user is-enabled "$SERVICO" 2>/dev/null) == enabled ]] && sim "Também parar de ligar junto com o PC?"; then
    systemctl --user disable "$SERVICO" -q && ok "Não liga mais sozinho."
  fi
}

log() {
  titulo "Log"
  local qual
  qual=$(escolher "Qual?" "Últimas 40 linhas" "Acompanhar ao vivo (Ctrl+C para sair)" "Só avisos e erros (último dia)") || return
  case $qual in
    Últimas*) journalctl --user -u "$SERVICO" -n 40 --no-pager -o cat ;;
    Acompanhar*) journalctl --user -u "$SERVICO" -f -o cat ;;
    *) journalctl --user -u "$SERVICO" --since "-1 day" --no-pager -o cat | grep -E '"level":"(WARN|ERROR)"' | tail -40 || ok "Nenhum aviso." ;;
  esac
}

atualizar() {
  titulo "Atualizar (compila o código atual e reinicia)"
  compilar || return
  instalar_binario
  if rodando; then
    systemctl --user restart "$SERVICO" && sleep 2
    rodando && ok "Atualizado e reiniciado." || erro "Não voltou. Veja o log (opção 4)."
  else
    ok "Instalado. Está desligado: use a opção 2 para ligar."
  fi
}

testes() {
  titulo "Testes"
  local qual
  qual=$(escolher "Quais?" "Backend (Go)" "Frontend (Vitest + tipos)" "Tudo (antes de entregar uma fase)") || return
  banco_no_ar || return
  case $qual in
    Backend*) (cd backend && go test ./...) ;;
    Frontend*) (cd frontend && npm test && npm run typecheck) ;;
    *) (cd backend && go vet ./... && go test -count=1 ./...) && (cd frontend && npm test && npm run typecheck) ;;
  esac && ok "Testes ok." || erro "Algum teste falhou."
}

usuarios() {
  titulo "Usuários do painel"
  [[ -x bin/netguardd ]] || compilar || return
  banco_no_ar || return
  carregar_env || { erro "Falta $ENV (rode scripts/primeira-vez.sh)."; return; }
  docker exec infra_pg psql -U netguard -d netguard -tAc \
    "SELECT '  ' || nome || '  (criado em ' || to_char(criado_em, 'DD/MM/YYYY') || ')' FROM usuarios ORDER BY nome" 2>/dev/null
  local qual nome
  qual=$(escolher "O quê?" "Criar usuário" "Trocar senha (derruba as sessões abertas)" "Voltar") || return
  [[ $qual == Voltar ]] && return
  read -rp "Nome: " nome || return
  if [[ $qual == Criar* ]]; then bin/netguardd usuario criar "$nome"; else bin/netguardd usuario senha "$nome"; fi
}

firewall() {
  titulo "Firewall (UFW)"
  printf 'Libera a porta %s só para a rede de casa (10.0.0.0/24). Pede a senha do sudo.\n' "$PORTA"
  sim "Continuar?" s || return
  scripts/liberar-firewall.sh
}

OPCOES=(
  "Estado"
  "Ligar"
  "Desligar"
  "Ver log"
  "Atualizar (depois de mudar o código)"
  "Rodar testes"
  "Usuários (criar / trocar senha)"
  "Firewall: liberar o painel para a rede de casa"
)

while true; do
  printf '\n%sNetGuard%s  %s%s%s\n' "$n" "$f" "$d" "$RAIZ" "$f"
  if rodando; then printf '  %s● ligado%s  http://%s:%s\n' "$v" "$f" "$(ip_casa)" "$PORTA"; else printf '  %s○ desligado%s\n' "$r" "$f"; fi
  for i in "${!OPCOES[@]}"; do printf '  %s%d)%s %s\n' "$c" $((i + 1)) "$f" "${OPCOES[$i]}"; done
  printf '  %s0)%s Sair\n' "$c" "$f"
  read -rp "> " op || exit 0
  case $op in
    1) estado ;;
    2) ligar ;;
    3) desligar ;;
    4) log ;;
    5) atualizar ;;
    6) testes ;;
    7) usuarios ;;
    8) firewall ;;
    0 | '') exit 0 ;;
    *) echo "Opção inválida."; continue ;;
  esac
  pausa
done
