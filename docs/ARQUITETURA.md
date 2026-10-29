# Arquitetura

Como o NetGuard está construído hoje (fases 1 a 3). O que ainda é plano está marcado.

## Visão geral

```
              navegador (rede de casa) ── HTTP, consulta a cada 10–60 s
                              │
┌──────────────────── netguardd (Go, 1 processo, sem root, sem Docker) ────────────────────┐
│ api ─ auth (sessões, CSRF, limite) ─ frontend Vue embutido                                │
│  │                                                                                        │
│  ├─ monitor ─► diagnostico ─► incidentes        a cada 10 s (configurável)                │
│  ├─ velocidade (medidor + agendador)            3×/dia + botão                             │
│  ├─ wifi (coletor nmcli)                        a cada 1 min                               │
│  ├─ dispositivos (coletor ip neigh + procura)   a cada 1 min; procura a cada 5 min         │
│  ├─ ajustes (gerente)                           tela de Configurações → avisa os módulos   │
│  └─ armazenamento (pgx, migrações, limpeza)                                                │
└──────────────────────────────────────────────────┬───────────────────────────────────────┘
                                                   ▼
                          PostgreSQL (database netguard, usuário netguard)
```

- **Um processo só.** Cada módulo é um pacote Go com interfaces pequenas; os testes trocam as
  partes de rede por versões falsas.
- **Nativo** (serviço systemd do usuário): o Docker rootless deste PC não enxerga a rede de casa.
- **Sem root:** ping pelo `net.ipv4.ping_group_range`; vizinhos e Wi-Fi por comandos de leitura
  (`ip -j neigh`, `nmcli`), com argumentos fixos.
- **Tempo real por consulta periódica** (10 s na Visão geral). SSE ficou de fora: não fez falta.

## Pacotes (`backend/internal/`)

| Pacote | Faz |
|---|---|
| `config` | Lê o ambiente (`NETGUARD_*`): endereço, banco, interface, intervalo padrão, redes aceitas. |
| `ajustes` | Configurações da tela: validação (`Validar`), padrão de fábrica, e o `Gerente`, que guarda a atual, grava e avisa os módulos (`AoMudar`). |
| `auth` | Usuários (argon2id), sessões (só o hash do token no banco), limite de 5 tentativas/min por IP. |
| `api` | Rotas, validação de entrada, middlewares (rede local, CSRF, cabeçalhos, log, pânico). |
| `monitor` | Ciclo de medição: interface, gateway (ping), IPs de fora (ping, TCP 443 de reserva), DNS do sistema × DNS de comparação, sites HTTPS. |
| `diagnostico` | Função pura: resultado do ciclo → estado (`ok`/`degradado`/`fora`) e causa provável. |
| `incidentes` | Máquina de estados das quedas: abre com 3 ciclos fora, fecha com 2 bons. |
| `velocidade` | Medidor (Cloudflare, 4 conexões ↓ / 2 ↑) e agendador (horários espalhados a partir das 7h). |
| `wifi` | Lê sinal, canal e ligação pelo `nmcli` (o driver `wl` não informa pelo kernel); `Vigia` busca as redes vizinhas a cada 30 min e analisa os canais (`Analisar24`, `Analisar5`) sobre a média das últimas 6 buscas. |
| `dispositivos` | Vizinhos com estado (`ip -j neigh`), procura ativa (1 ping por endereço), nome pelo DNS reverso do roteador, fabricante pela lista OUI (`hwdata`). |
| `saude` | Função pura: tudo o que foi medido → nota, itens com estado/prova/recomendação e a lista do que fazer. |
| `computador` | Lê memória, pressão (PSI), carga, disco, temperatura e tempo ligado de /proc e /sys a cada minuto; guarda a última hora na memória. |
| `armazenamento` | PostgreSQL: migrações embutidas com trava, consultas parametrizadas, limpeza do antigo. |
| `web` | Serve o frontend compilado (embutido no binário). |

## Diagnóstico (ordem das causas)

| Condição | Estado · causa |
|---|---|
| Interface sem link, IP ou rota | fora · rede local (este aparelho) |
| Gateway não responde | fora · roteador |
| Nenhum IP de fora responde (nem por TCP 443) | fora · externo (provedor **ou** roteador) |
| DNS configurado falha, o de comparação responde | fora · DNS configurado |
| Os dois DNS falham | fora · DNS |
| Algum site falha | degradado · serviço |
| Perda média dos de fora acima do limite | degradado · perda |
| Latência média dos de fora acima do limite | degradado · lentidão |

Durante um teste de velocidade, latência e perda não contam (o próprio teste enche a fila);
queda continua contando. Durante a busca de redes Wi-Fi o ciclo inteiro é descartado: a placa sai
do canal por 1–2 s e tudo falha junto.

## Dispositivos: como decide "na rede"

1. A cada minuto lê a tabela de vizinhos **com o estado**. Só `REACHABLE`/`DELAY`/`PROBE` contam:
   o kernel guarda entradas `STALE` por horas, e contar essas mostraria todo mundo sempre presente.
2. Com a procura ligada (ou "Procurar agora"), antes de ler manda 1 ping para cada endereço da
   rede (até 1024, 16 por vez). Quem existe responde ao ARP, mesmo bloqueando ping.
3. A procura vai em blocos de 64 endereços e grava quem já respondeu depois de cada bloco, para a
   tela mostrar o progresso e os aparelhos surgindo. A primeira procura de todas é "retrato" em todos
   os blocos (marcada só no fim).
4. Cada aparelho visto estende a aparição atual; visto depois de 10 min sem aparecer abre outra.
5. Aparelho que nunca existiu vira "novo" (aviso). Exceções: a primeira coleta e a primeira
   procura de todas (retrato inicial da casa) e o próprio PC.

## Wi-Fi da vizinhança: como escolhe o canal

- **Disputa** de um canal = soma de (sinal/100)² dos vizinhos: uma rede a 90% (0,81) pesa como cinco a 40%.
- **2,4 GHz:** canais de 20 MHz se sobrepõem até 4 de distância (peso cai com a distância); só 1, 6 e
  11 são recomendados (não se sobrepõem).
- **5 GHz:** blocos de 80 MHz (36–48, 52–64, 100–112, 116–128, 132–144, 149–161); redes no mesmo bloco
  disputam; os do meio são DFS e não são recomendados.
- **Disputa** pouca < 0,25 ≤ média < 0,8 ≤ muita.
- Só manda trocar se o melhor tiver menos de 60% da disputa atual e a diferença passar de 0,15.
- Os rádios do próprio roteador (mesmo BSSID tirando o último byte) não contam como vizinhos e são
  lembrados por 24 h quando uma busca não os traz.
- Sem gravar no banco: as últimas 6 buscas ficam na memória (recomeça ao reiniciar).

## Saúde: como avalia

Cada ponto tem estado (`ok`, `dica`, `atencao`, `problema`, `sem_dados`), resumo, **prova** e
recomendação. A nota começa em 100 e desconta 15 × peso por problema e 6 × peso por atenção (dica
não desconta). Sem nota se estabilidade ou tempo de resposta estiverem sem dados, ou se metade dos
pontos estiver. Limites principais:

| Ponto | Bom | Atenção | Problema |
|---|---|---|---|
| Quedas (7 dias, sem contar a placa do PC) | 0 | 1–2 e < 30 min | 3+ ou 30 min+ |
| Tempo de resposta (mediana 24 h) | < 60 ms | < 120 ms | 120 ms+ |
| Perda (24 h) | < 0,5% | < 2% | 2%+ |
| Horário de pico (≥ 36 h medidas) | pior hora < 1,8× o normal | 1,8×+ | — |
| Velocidade × plano | ≥ 60% (Wi-Fi) / 85% (cabo) | ≥ 70% disso | menos |
| Fila cheia (piora com a internet ocupada) | < 30 ms | < 100 ms | 100 ms+ |
| Sinal do Wi-Fi (24 h) | média ≥ 60% e pior ≥ 30% | média ≥ 40% | menos |
| Memória | > 15% livre e pressão < 1% | < 15% ou pressão 1%+ | < 5% ou pressão 10%+ |
| Processador (carga / núcleos) | < 70% | < 150% | 150%+ |
| Disco | < 80% | 80%+ (ou esperando 10%+) | 90%+ |
| Temperatura da CPU (máx. da hora) | < 75 °C | < 85 °C | 85 °C+ |

Números com vírgula no texto (endereços como 8.8.8.8 ficam intactos).

## Banco de dados

| Tabela | Conteúdo | Guarda |
|---|---|---|
| `usuarios`, `sessoes` | login (argon2id), sessões (hash SHA-256 do token) | — |
| `auditoria` | quem, quando, ação, alvo, detalhe, IP | sempre |
| `verificacoes` | cada sonda de cada ciclo | 35 dias |
| `ciclos` | estado e causa de cada ciclo (base da disponibilidade) | 400 dias |
| `monitor_execucoes` | períodos com o monitor rodando (cobertura) | sempre |
| `incidentes` | quedas: início, fim, causas | sempre |
| `configuracao` | ajustes da tela (JSON) e marcas internas | — |
| `testes_velocidade` | cada teste, com a resposta parada, baixando e enviando | sempre |
| `wifi_amostras` | sinal, canal, ligação a cada minuto | 400 dias |
| `dispositivos` | aparelhos (MAC único), apelido, notas, conhecido, novo | sempre |
| `aparicoes` | períodos de presença de cada aparelho | 400 dias |

Migrações em `internal/armazenamento/migrations/000N_*.sql`, aplicadas ao iniciar, só avançam.

## API (`/api/v1`)

Tudo exige login, menos `saude` e `auth/entrar`. Escrita exige o cabeçalho `X-NetGuard: 1`.

| Método e caminho | Faz |
|---|---|
| `GET /saude` | banco ok, última medição, versão |
| `POST /auth/entrar` · `POST /auth/sair` · `GET /auth/eu` | sessão |
| `GET /visao-geral` | tudo da tela inicial |
| `GET /internet/historico?periodo=1h\|6h\|24h\|7d\|30d` | séries para os gráficos (nulo onde não mediu) |
| `GET /internet/disponibilidade` | 24 h, 7 d e 30 d com cobertura |
| `GET /quedas?periodo&limite` | quedas |
| `GET /velocidade` · `POST /velocidade/testar` | testes, gasto, próximo horário · teste agora (409 rodando, 429 cedo) |
| `GET /wifi?periodo` | leitura atual e histórico do sinal |
| `GET /wifi/vizinhanca` · `POST /wifi/vizinhanca/buscar` | redes por perto, ocupação por canal, comparação e recomendação (BSSID dos vizinhos não sai da API) · buscar agora (409/429) |
| `GET /configuracao` · `PUT /configuracao` | ajustes atuais e de fábrica · salvar (400 com a lista de erros) |
| `GET /dispositivos` · `GET /dispositivos/{id}` · `PATCH /dispositivos/{id}` | lista e resumo (com o progresso da procura em andamento) · ficha com aparições · apelido, notas, conhecido, "já vi" |
| `GET /dispositivos/movimentos?periodo` | quem entrou e quem saiu (saída só depois da janela de 10 min) |

A lista traz `presenca_7d` (entradas e segundos) de cada aparelho; a ficha traz `resumo_7d` (entradas,
tempo, média por vez e hora típica de chegada, mediana com 3+ entradas).
| `POST /dispositivos/varrer` | procurar agora (409/429) |
| `GET /aparelhos-novos?periodo` | aparelhos que surgiram |
| `GET /saude-da-rede` | relatório da tela Saúde (nota, itens, o que fazer) |
| `GET /computador` | leitura atual da máquina e o pior da última hora |

Dado que falta vem como `{"disponivel": false, "motivo": "..."}`, nunca como zero.

## Segurança

| Regra | Como |
|---|---|
| Senha | argon2id (19 MiB, 2 passadas); usuário só nasce pelo terminal |
| Sessão | cookie `HttpOnly`, `SameSite=Strict`, 7 dias renovando; token aleatório de 32 bytes, só o hash no banco; trocar a senha derruba todas |
| CSRF | escrita exige `X-NetGuard: 1`; confere `Origin` e `Sec-Fetch-Site` |
| Só rede local | recusa IP fora das redes privadas (`NETGUARD_REDES`); firewall libera 8095 só para 10.0.0.0/24 |
| Entrada | JSON estrito (campo desconhecido = 400), corpo até 64 KB, cada campo validado (IPs, URLs só http/https sem senha, tamanhos, caracteres de controle) |
| Comandos | nenhum vindo do navegador; `ip` e `nmcli` com argumentos fixos e a interface validada |
| Auditoria | login, falhas, configurações (com antes/depois), testes de velocidade, procuras, mudanças em aparelhos |
| Privacidade | não captura tráfego; só IP, MAC, nome que o roteador conhece e as próprias medições |
| Ações que mudam algo | confirmação na tela (salvar configurações, testar velocidade, ligar/rodar procura) |

O painel é HTTP na rede local (como o Portainer); `NETGUARD_COOKIE_SEGURO=true` quando houver HTTPS.

## Frontend (`frontend/src/`)

Vue 3 + TypeScript + Vite, sem biblioteca de componentes. `views/` (uma por tela),
`components/` (cartão, selo, indisponível, carregando, erro, confirmar, gráfico, seções),
`composables/` (sessão, consulta periódica que pausa com a aba escondida), `api/` (cliente e
tipos). Gráficos com uPlot e paleta validada para daltonismo nos temas claro e escuro.

## Testes

- Go (`go test ./...`, com `-race`): 116 testes. Banco em schema temporário do `netguard_test`;
  rede simulada (pinger, DNS, HTTP e vizinhos falsos). Os únicos que tocam a rede de verdade são
  só de leitura: ping em 127.0.0.1, `nmcli` e `ip neigh`, e pulam onde não houver.
- Vitest: 43 testes de telas, com a API simulada e o gráfico substituído por um dublê (sem canvas).
- Antes de entregar cada fase: teste de fumaça com o binário num schema descartável e prints em
  navegador real (claro, escuro, celular).

## Planejado (ainda não existe)

- **Pi-hole** (fase 4), **consumo e alertas fora do painel** (fase 5), **estabilização** (fase 6).
