# Dependências

Fase 1 instalada com ok do dono em 2026-10-29 (versões abaixo). O resto continua esperando ok
(regra da casa: avisar antes de instalar).

## Instalado (fase 1)

| O que | Versão | Observação |
|---|---|---|
| Go (pacote do Arch, instalado pelo dono) | 1.27.3 | |
| `github.com/jackc/pgx/v5` | 5.11.0 | driver PostgreSQL |
| `golang.org/x/crypto` | 0.58.0 | argon2id |
| `golang.org/x/net` | 0.62.0 | ICMP sem root |
| `golang.org/x/term` | 0.47.0 | ler a senha no terminal sem mostrar (já vinha junto do x/crypto) |
| `vue` · `vue-router` | 3.5.44 · 5.4.0 | |
| `vite` · `@vitejs/plugin-vue` | 8.3.5 · 6.0.10 | só desenvolvimento |
| `typescript` · `vue-tsc` | **6.0.3** · 3.3.13 | TypeScript 7 (o novo, em Go) não funciona com o vue-tsc ainda: ficar na 6 |
| `vitest` · `@vue/test-utils` · `jsdom` | 5.0.5 · 2.5.1 · 30.1.3 | só desenvolvimento |

| `uplot` | 1.6.32 | gráficos (fase 2), MIT, ~50 KB |

`npm audit`: 0 vulnerabilidades.

## Ferramentas do sistema usadas (já vinham instaladas)

| O que | Para quê | No Pi |
|---|---|---|
| `nmcli` (NetworkManager) | sinal e canal do Wi-Fi | Raspberry Pi OS atual usa NetworkManager; sem ele, a parte de Wi-Fi fica "indisponível" |
| `ip` (iproute2) | tabela de vizinhos com estado | vem em todo Linux |
| `/usr/share/hwdata/oui.txt` (pacote `hwdata`) | fabricante pelo MAC | instalar `hwdata` ou `ieee-data`; sem ele, fica sem fabricante |
| `speed.cloudflare.com` (serviço, não instalado) | teste de velocidade | igual |
| Regra do polkit `/etc/polkit-1/rules.d/50-netguard-busca-wifi.rules` (criada pelo dono com `scripts/permitir-busca-wifi.sh`) | deixar o usuário buscar redes Wi-Fi sem estar logado na tela | criar de novo no Pi, se usar Wi-Fi |

## Ainda não instalado (cada um espera ok)

| O que | Para quê | Fase | Observação |
|---|---|---|---|
| `github.com/vishvananda/netlink` | vizinhos IPv6, Wi-Fi via nl80211 | 2–3 | começar lendo `/proc` e `/sys`; só se faltar |
| Tabela OUI da IEEE (arquivo) | fabricante pelo MAC | 3 | o sistema já tem `/usr/share/hwdata/oui.txt` (pacote `hwdata`); no Pi, embutir uma cópia |
| `github.com/gosnmp/gosnmp` | SNMP do roteador | 5 | só se o roteador tiver SNMP |
| Pi-hole v6 | bloqueio de anúncios | 4 | só no Pi; mexe na porta 53 / `systemd-resolved` |
| Playwright | conferir telas em navegador real | 6 | opcional; hoje os prints usam o `chromium` já instalado, por um script de 40 linhas no scratchpad |

Fora de propósito: framework web (a `net/http` resolve), ORM, biblioteca de migração (runner
próprio), WebSocket (SSE na biblioteca padrão), biblioteca de componentes, Pinia, axios.
