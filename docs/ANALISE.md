# Análise: riscos e limitações

Feita em 2026-10-29, olhando o pedido ([ESPECIFICACAO.md](ESPECIFICACAO.md)), este PC e os outros
projetos do dono.

## Este PC hoje

| Item | Valor | Por que importa |
|---|---|---|
| Sistema | CachyOS (Arch), kernel 7.3 | alvo inicial do pedido |
| Memória | 7,6 GB (o oomd já matou sessão com tudo aberto) | o serviço tem que ser leve |
| Rede | só `wlan0` (Wi-Fi, driver `wl`); `enp3s0f0` (cabo) existe, sem cabo | Wi-Fi instável, ver risco 2 |
| IP / gateway | 10.0.0.209 / 10.0.0.1, rede 10.0.0.0/24 | alvo da descoberta |
| Docker | 29.4, **rootless** | ver risco 3 |
| Ping sem root | `net.ipv4.ping_group_range = 0 2147483647` | ICMP funciona sem `CAP_NET_RAW` |
| DNS local | `systemd-resolved` em 127.0.0.53 e 127.0.0.54 | conflita com Pi-hole na porta 53 |
| Go | **não instalado** | precisa de ok para instalar |
| Node | 22.23 | frontend ok |
| Postgres | infra compartilhada, `postgres:15`, `localhost:5432` | criar database `netguard` com `new-project.sh` |
| Ligado sempre? | não: só o Portainer sobe no boot | ver risco 1 |

## Riscos e limitações

### 1. Monitor que desliga erra a disponibilidade

Disponibilidade de 7 e 30 dias pede um aparelho ligado o tempo todo. Neste PC isso não acontece.

**Medida:** o serviço registra os próprios períodos ligado (batimento a cada ciclo). Tempo em que o
monitor estava desligado aparece como **sem dados** e não entra na conta, nem como queda nem como
tempo no ar. A tela mostra "cobertura do período" (ex.: "dados de 62% dos últimos 7 dias").
O destino certo é o Raspberry Pi; este PC serve para desenvolver e testar.

### 2. Wi-Fi instável deste PC parece queda da internet

O driver `wl` trava (é o que o `wifi-watchdog` resolve). Quando a placa trava, todas as
verificações falham juntas.

**Medida:** antes de tudo, cada ciclo confere a interface (link, IP, rota padrão). Interface caída
ou sem rota = causa **"rede local (este aparelho)"**, nunca "internet". O monitor de verdade deve
ficar no cabo.

### 3. Docker rootless não enxerga a rede de casa

No Docker rootless, `network_mode: host` é a rede do namespace do usuário (rootlesskit), não a do
PC. ARP, tabela de vizinhos, ping no gateway e contadores da `wlan0` ficam errados ou vazios.

**Medida:** o `netguardd` roda **nativo** (serviço systemd do usuário). No Docker fica só o
postgres (o da infra compartilhada). O `compose.yml` do projeto serve para o Pi, onde o Docker
pode ser o normal (com root) e `network_mode: host` funciona.

### 4. Pi-hole num PC que desliga derruba a internet da casa

Se o roteador mandar o DNS para este PC e o PC desligar, nenhum aparelho resolve nome. Pi-hole
só faz sentido num aparelho sempre ligado (o Pi). Também:

- precisa liberar a porta 53 do `systemd-resolved` (`DNSStubListener=no`), o que muda o DNS do
  próprio PC — mudança de sistema, só com ok;
- a API mudou na **v6** (`/api/auth` com sessão, endpoints novos); a v5 (`api.php`) não serve.
  A integração é só para a v6, conferida na doc da versão instalada;
- para valer para a casa toda, o DNS tem que ser trocado **no roteador** ou aparelho por
  aparelho. O NetGuard só orienta, nunca troca sozinho.

Bloqueio por DNS não pega anúncio que vem do mesmo domínio do conteúdo (YouTube, por exemplo).
Isso vai escrito na própria tela.

### 5. Consumo por aparelho provavelmente não vai dar

Sem o roteador ajudar, este PC só vê o tráfego **dele**. Medir cada aparelho exige uma destas:

| Fonte | Precisa de |
|---|---|
| SNMP (IF-MIB, contadores da WAN) | roteador com SNMP ligado → dá o total da casa, não por aparelho |
| Contadores por cliente do roteador | firmware que mostra isso (OpenWrt, alguns Mikrotik/TP-Link) |
| NetFlow/IPFIX | roteador que exporta fluxos (raro em roteador de operadora) |
| Aparelho no meio do caminho | Pi como roteador/ponte — muda a rede, fora do escopo agora |

Roteador de operadora quase nunca tem nada disso. A tela **Consumo de rede** começa com o tráfego
da interface deste aparelho, com rótulo claro, e as outras visões aparecem como "indisponível:
precisa de X". Nunca divide o total entre os aparelhos.

### 6. A lista de aparelhos nunca é completa

- Aparelho só entra na tabela ARP depois de trocar pacote com alguém; aparelho dormindo some.
- Roteador com **isolamento de clientes** no Wi-Fi (comum em rede de visitante) esconde os outros.
- **MAC aleatório** (Android e iPhone por rede): o mesmo celular pode aparecer com outro MAC, e o
  fabricante não é identificável. O bit "administrado localmente" do MAC denuncia isso; esses
  aparelhos ganham a marca "MAC aleatório" e o alerta de "novo aparelho" leva isso em conta.
- IPv6 tem vizinhos próprios; entra depois do IPv4.

A tela sempre diz "aparelhos **observados**", nunca "conectados".

**Visto na prática (fase 3):** o kernel guarda entradas antigas como `STALE` por horas (abaixo de
128 entradas nem limpa), e `/proc/net/arp` mostra todas como completas. Por isso a presença usa o
estado de `ip -j neigh`. Sem a procura ativa só apareciam 3 de 9; com ela, os 9 em ~16 s.

### 7. Varredura ativa precisa de autorização

Varredura passiva (ler a tabela de vizinhos do kernel) não envia nada. A ativa (ping em toda a
/24 para popular a tabela) manda pacote para os outros aparelhos. Fica **desligada** por padrão,
liga só nas Configurações, com confirmação, e vai para a auditoria. Nada de nmap/varredura de
portas.

### 8. Postgres pesa no Pi

No PC, o postgres da infra já está lá. No Pi, um postgres só para isso gasta ~50–100 MB com
ajuste. Aceitável num Pi de 2 GB ou mais; ruim num de 1 GB. Ver [DECISOES.md](DECISOES.md) (D5).

### 9. Volume do histórico

Uma verificação a cada 10 s em ~6 alvos dá ~50 mil linhas por dia. Sem limpeza, o banco cresce
para sempre e o Pi (cartão SD) sofre.

**Medida:** resultados brutos ficam 7 dias; resumo por minuto, 90 dias; resumo por hora, para
sempre. Incidentes e eventos nunca são apagados.

### 10. "Problema da operadora" não dá para provar daqui

Sem acesso ao roteador, "gateway responde, mas nada externo responde" pode ser o provedor **ou**
o lado WAN do roteador. A causa fica "conectividade externa (provedor ou roteador)", não
"operadora".

## O que já existe e onde o NetGuard se encaixa

| Já existe | Cobre | No NetGuard |
|---|---|---|
| [Sentinela](../../sentinela) | sites no ar, vistos de fora (VPS e sondas) | não repetir; no máximo mandar alerta pelo mesmo canal |
| `wifi-watchdog` | destrava o Wi-Fi deste PC | o NetGuard só registra a queda como "rede local" |
| Uptime Kuma (catálogo open source) | verificações HTTP/ping com página de status | não copiar; o foco aqui é diagnóstico e aparelhos |
| Painel do Pi-hole | estatísticas e listas | o NetGuard mostra o resumo e as ações mais usadas, não refaz tudo |

O que justifica o projeto: **causa provável de cada queda**, **inventário de aparelhos com
histórico** e tudo numa tela só, leve o bastante para um Pi.
