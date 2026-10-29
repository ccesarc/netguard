# NetGuard

Monitor da rede de casa: diz se a internet caiu e **de quem é a culpa** (Wi-Fi deste aparelho,
roteador, provedor, DNS ou um site), mede velocidade e Wi-Fi, mostra **quem está na rede** e
guarda o histórico com gráficos. Roda neste PC; o destino final é um Raspberry Pi sempre ligado.

**Estado (2026-10-29):** fases 1, 2 e 3 prontas, mais Wi-Fi da vizinhança e Saúde da rede e do
computador. Pi-hole (fase 4) espera o Raspberry Pi; alertas e backup ficaram para depois.

> **Feito para a rede de casa.** O painel só aceita acesso de redes privadas: não exponha a porta
> na internet (nada de redirecionar porta no roteador). Os nomes de rede, MACs e aparelhos que
> aparecem nos testes e na documentação são fictícios.

| Tela | O que tem |
|---|---|
| Visão geral | faixa da saúde, internet, roteador, DNS, este aparelho, gráfico da última hora, disponibilidade, velocidade, aparelhos |
| **Saúde** | nota de 0 a 100, o que fazer e a prova de cada ponto da rede e do computador (memória, disco, temperatura) |
| Internet | disponibilidade 24 h/7 d/30 d, velocidade (com "Testar agora"), Wi-Fi, **Wi-Fi da vizinhança e melhor canal**, latência, perda, quedas, cada verificação |
| Dispositivos | aparelhos na rede, procura, aviso de aparelho novo, apelidos, quando cada um esteve na rede |
| Histórico de eventos | quedas e aparelhos que apareceram |
| Configurações | destinos testados, intervalos, limites, teste de velocidade, procura de aparelhos |
| Consumo de rede · DNS e anúncios · Alertas | ainda "em breve" (fases 4 e 5) |

**Como usar cada tela:** [docs/USO.md](docs/USO.md).

## Documentos

| Arquivo | O que tem |
|---|---|
| [docs/USO.md](docs/USO.md) | **Guia de uso**: cada tela, o que cada causa quer dizer, perguntas frequentes |
| [docs/ARQUITETURA.md](docs/ARQUITETURA.md) | Como está construído: pacotes, diagnóstico, banco, API, segurança, testes |
| [docs/ROADMAP.md](docs/ROADMAP.md) | Fases, o que foi feito em cada uma e o que falta |
| [docs/DECISOES.md](docs/DECISOES.md) | Perguntas em aberto e decisões tomadas, com o porquê |
| [docs/ANALISE.md](docs/ANALISE.md) | Riscos e limitações (e o que se confirmou na prática) |
| [docs/DEPENDENCIAS.md](docs/DEPENDENCIAS.md) | O que foi instalado, ferramentas do sistema usadas, o que espera ok |
| [docs/MEDICOES.md](docs/MEDICOES.md) | Primeira medição da rede, à mão |
| [docs/ESPECIFICACAO.md](docs/ESPECIFICACAO.md) | Pedido original |
| [docs/HISTORICO.md](docs/HISTORICO.md) | Diário das sessões |

## Como funciona

- Um programa só, `netguardd` (Go), roda **direto no Linux, sem root e sem Docker**: mede em
  segundo plano, serve a API e as telas (Vue, embutido no binário).
- Banco: PostgreSQL. Neste PC, o database `netguard` (usuário próprio `netguard`) no postgres da
  infra compartilhada (`~/projects/infra`).
- Ping sem root pelo `net.ipv4.ping_group_range` do kernel (já liberado no Arch/CachyOS).

## Pastas

```
backend/    Go: cmd/netguardd + internal/{api,auth,config,ajustes,monitor,diagnostico,incidentes,
            velocidade,wifi,dispositivos,saude,computador,armazenamento,web}
frontend/   Vue 3 + TypeScript + Vite (compila para backend/internal/web/dist/app)
deploy/     netguardd.service (systemd do usuário) e netguard.env.exemplo
scripts/    primeira-vez, menu, compilar, liberar-firewall, testar-busca-wifi, permitir-busca-wifi
docs/
```

## Uso do dia a dia (terminal)

```bash
painel            # → NetGuard → "Abrir o menu do próprio projeto"
# ou
~/projects/netguard/scripts/menu.sh
```

Menu: 1 Estado · 2 Ligar · 3 Desligar · 4 Log · **5 Atualizar** (depois de mudar o código) ·
6 Testes · 7 Usuários · 8 Firewall.

## Primeira vez

```bash
cd ~/projects/netguard
./scripts/primeira-vez.sh         # banco, compila, configuração, usuário e (se quiser) serviço
./scripts/liberar-firewall.sh     # para abrir pelo celular (porta 8095 só para 10.0.0.x)
```

Já feito neste PC em 2026-10-29: Go instalado, usuário `netguard` e databases `netguard` e
`netguard_test` no postgres da infra; URLs em `~/.config/netguard/banco_url` e `banco_url_teste`
(permissão 600); usuário do painel criado; serviço instalado; firewall liberado.

O postgres da infra precisa estar no ar (`cd ~/projects/infra && docker compose up -d postgres`).
Se ele subir depois, o `netguardd` fica tentando conectar até conseguir.

## Compilar

```bash
scripts/compilar.sh                 # bin/netguardd para este PC
GOARCH=arm64 scripts/compilar.sh    # para o Raspberry Pi (64 bits)
```

## Instalar como serviço (manual)

```bash
install -Dm755 bin/netguardd ~/.local/bin/netguardd
install -Dm644 deploy/netguardd.service ~/.config/systemd/user/netguardd.service
systemctl --user daemon-reload && systemctl --user enable --now netguardd
journalctl --user -u netguardd -f
```

Para rodar com o PC ligado sem ninguém logado: `sudo loginctl enable-linger $USER`.

## Configuração

`~/.config/netguard/netguard.env` (modelo em `deploy/netguard.env.exemplo`) só tem o que precisa
existir antes do banco: endereço/porta, arquivo da URL do banco, interface, intervalo padrão, nível
de log, redes aceitas. O resto muda pela tela de **Configurações** e vale na hora.

## Desenvolver

```bash
# terminal 1: backend
cd backend && set -a && source ~/.config/netguard/netguard.env && set +a && go run ./cmd/netguardd
# terminal 2: frontend com recarga automática em http://localhost:5180
cd frontend && npm run dev
```

## Testes

```bash
cd backend && go test ./...          # 116 testes; banco em schema temporário do netguard_test
cd frontend && npm test              # 43 testes (Vitest)
cd frontend && npm run typecheck
```

Os testes não derrubam a internet: a rede é simulada. Os que tocam a rede de verdade só leem
(ping em 127.0.0.1, `nmcli`, `ip neigh`) e pulam onde não houver.

## Saúde

`GET /api/v1/saude` (sem login): `200` com `banco: ok` e a última medição, ou `503` com o banco fora.

## Problemas comuns

| Sintoma | Causa provável |
|---|---|
| Painel não abre | serviço desligado: menu → 1 (Estado) / 2 (Ligar) |
| Abre no PC mas não no celular | firewall: `scripts/liberar-firewall.sh` |
| `banco indisponível, tentando de novo` no log | postgres da infra parado: `docker compose up -d postgres` em `~/projects/infra` |
| Tela sem as novidades depois de mudar o código | menu → 5 (Atualizar) e F5 no navegador |
| `sem permissão para ping` | `sysctl net.ipv4.ping_group_range` não inclui o grupo do usuário |
| Página "frontend não compilado" | binário feito sem `npm run build`: use `scripts/compilar.sh` |
| `acesso só pela rede local` | acesso de fora das redes privadas (ou de rede fora de `NETGUARD_REDES`) |
| Login bloqueado por 1 min | 5 senhas erradas seguidas do mesmo IP |
| Wi-Fi "indisponível" | sem NetworkManager, ou o aparelho está no cabo |
| Poucos aparelhos em Dispositivos | procura automática desligada: "Ligar procura" |
| Esqueci a senha | menu → 7 (Usuários) → Trocar senha |

## Licença

[MIT](LICENSE).
