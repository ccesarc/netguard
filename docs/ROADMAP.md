# Roadmap

Uma fase por vez. No fim de cada fase: suíte de testes, resumo (o que mudou, arquivos, resultado
dos testes, riscos pendentes, próximo passo) e linha no [HISTORICO.md](HISTORICO.md).

**Primeira entrega = fases 1 a 3, só com este PC** (o Raspberry Pi ainda vai ser comprado). As
fases 4 e 5 dependem do Pi e do roteador ([DECISOES.md](DECISOES.md)). O que já foi medido à mão
está em [MEDICOES.md](MEDICOES.md).

## Fase 0 — Análise e plano ✅

- [x] Pasta e documentação (esta pasta `docs/`).
- [x] Dono aprovou o Go e as dependências da fase 1 (2026-10-29).

## Fase 1 — Fundação ✅ (2026-10-29)

- Instalar Go (com ok), criar o database `netguard`, `git init`.
- Backend: `cmd/netguardd`, config, logs estruturados (`log/slog`), runner de migrações, `/saude`,
  login (usuário criado no terminal), sessões, CSRF, auditoria, encerramento limpo (SIGTERM).
- Frontend: Vite + Vue + TS, layout com menu lateral, login, componentes base (indicador, gráfico,
  indisponível, carregando, erro, confirmar).
- Primeira medição real: estado da interface e do gateway no painel (é pouco, mas é real).
- Serviço systemd do usuário (`deploy/netguardd.service`), sobe com o PC.

**Aceite:** `netguardd` sobe, aplica migrações, responde `/saude`; login funciona e telas sem login
redirecionam; o painel mostra interface e gateway medidos de verdade, e "indisponível" no resto;
testes de auth e migração passam; memória do serviço parado abaixo de 50 MB.

**Resultado:** tudo cumprido. Memória: 38 MB parado, ~50 MB depois de um login com
`GOMEMLIMIT=40MiB` (o argon2 usa 19 MiB por login de propósito). A meta original de 30 MB era
otimista para Go + pgx + HTTP; 50 MB cabe folgado num Pi. Serviço systemd escrito, não instalado
(espera ok do dono).

## Fase 2 — Monitoramento

**Parte A pronta (2026-10-29):** sondas de IPs de fora (ping, com TCP 443 de reserva), DNS
configurado × DNS de comparação, sites por HTTPS; diagnóstico com causa provável; quedas com
histerese (3 para abrir, 2 para fechar) e fechamento de queda deixada aberta quando o serviço
parou; cobertura do monitor; limpeza do bruto com mais de 35 dias; telas Visão geral (gráfico da
última hora), Internet (gráficos por período, disponibilidade 24 h/7 d/30 d, cada verificação,
quedas) e Histórico de eventos. Atualização a cada 10 s por consulta periódica (SSE ficou para a
parte B, se fizer falta).

**Parte B pronta (2026-10-29):** teste de velocidade (Cloudflare, 4 conexões de download e 2 de
upload, 3×/dia a partir das 7h + botão com confirmação, 2 min entre testes manuais, pula com a
internet fora, monitor não chama de lentidão a latência que o próprio teste causa); Wi-Fi pelo
nmcli a cada minuto (sinal, canal, ligação, redes no mesmo canal) com gráfico; tela de
Configurações (destinos, DNS de comparação, sites, intervalo, limites, teste de velocidade) que
vale na hora, com confirmação, validação e auditoria do que mudou.

Fica para depois, se precisar: resumos por minuto/hora (só se 30 dias ficar lento) e SSE.

**Fase 2 ✅**

- Sondas gateway, externo (ICMP + TCP 443), DNS (sistema + externo), HTTP.
- Diagnóstico com causa provável e histerese; incidentes; cobertura do monitor.
- Resumos por minuto/hora e limpeza dos brutos.
- Telas Visão geral, Internet e Histórico de eventos com gráficos e tempo real (SSE).
- Configuração de alvos, intervalos e limites pela tela.
- **Teste de velocidade agendado** (padrão 3× por dia + botão "testar agora"): download e upload por
  HTTPS (ex.: `speed.cloudflare.com`, sem instalar nada), com o volume gasto por teste visível na
  tela. Rótulo deixa claro que mede "até este aparelho" (Wi-Fi ou cabo).
- **Qualidade do Wi-Fi**: sinal, canal, faixa, velocidade da ligação, redes vizinhas no mesmo
  canal (`nl80211`/`iw`), com histórico — ajuda a separar "Wi-Fi ruim" de "internet ruim".

**Aceite:** desligar a placa Wi-Fi gera incidente "rede local (este aparelho)", não "internet";
alvo externo inválido gera "conectividade externa" nos testes; DNS falso quebrado gera "DNS";
disponibilidade de 24 h/7 d/30 d bate com os casos de teste e mostra cobertura; reiniciar o
serviço não perde nem duplica incidente aberto; nenhum teste depende da internet real.

## Fase 3 — Dispositivos ✅ (2026-10-29)

**Resultado:** tabela de vizinhos com estado (`ip -j neigh`: STALE não conta), procura ativa
(desligada de fábrica; "Ligar procura" e "Procurar agora" com confirmação), nome pelo DNS reverso
do roteador, fabricante pela lista OUI do `hwdata`, MAC aleatório marcado, aparições, apelido,
notas, "conhecido", aviso de aparelho novo (a primeira coleta e a primeira procura são o retrato
da casa, sem aviso), aparelhos novos no Histórico. Medido: a procura achou os 9 aparelhos da casa
em ~16 s; sem procura, 3. mDNS ficou de fora (o firewall bloqueia e o nome do roteador bastou).

Plano original:

- Leitura passiva dos vizinhos, aparições, fabricante (OUI), MAC aleatório.
- Nome pelo **DNS reverso no roteador** (funcionou na medição à mão) e, se der, mDNS.
- Apelido, confiável, notas; alerta de aparelho novo (no painel).
- Varredura ativa (ping na /24, devagar): o dono já autorizou em 2026-10-29, mas continua sendo
  uma opção na tela com confirmação e auditoria. Sem ela a lista fica quase vazia (2 de 9).
- Tela Dispositivos com aviso "aparelhos observados — a lista pode não estar completa".

**Aceite:** aparelhos da rede aparecem com IP/MAC/fabricante quando possível; aparelho que some
vira "não observado" e a aparição fecha; troca de IP não cria aparelho novo; MAC aleatório é
marcado; aparelho novo gera alerta uma vez; varredura ativa não roda sem ter sido ligada.

## Extra — Wi-Fi da vizinhança e melhor canal ✅ (2026-10-29)

**Resultado:** regra do polkit criada pelo dono (`scripts/permitir-busca-wifi.sh`); busca a cada
30 min + "Buscar agora"; barras de ocupação por canal (teu canal em destaque, DFS esmaecido);
comparação dos canais candidatos do melhor para o pior, **com o porquê** (quais redes, o sinal e se é
forte, média ou fraca); recomendação pela **média das últimas 6 buscas** (uma busca só mudava de
ideia: 18 redes numa, 23 na seguinte); lembra os rádios do próprio roteador quando um não responde.
Resultado na casa: 5 GHz no 48 ótimo (só um vizinho fraco); 2,4 GHz disputado (até 23 redes), o 11
aparece como o menos ruim, o 6 tem o Vizinho1 a ~92%.

Plano original:

Mostrar as redes Wi-Fi vizinhas (nome, canal, sinal; informação que todo roteador anuncia
publicamente) e recomendar o canal mais livre em 2,4 e 5 GHz. Só recomenda: trocar o canal é no
roteador (sem a senha dele, o dono não troca).

**Bloqueio encontrado:** o NetworkManager só deixa pedir busca de redes a quem está logado na tela
("not authorized" para o serviço em segundo plano). Sem busca, só aparece a própria rede.

Passos:
1. Dono roda `scripts/testar-busca-wifi.sh` no Konsole: mostra se o driver `wl` enxerga os vizinhos.
2. Se enxergar: regra do polkit permitindo `org.freedesktop.NetworkManager.wifi.scan` ao usuário
   (um arquivo em `/etc/polkit-1/rules.d/`, com `sudo`, com ok do dono).
3. Busca a cada 15 min (a busca pausa o Wi-Fi por ~2 s), tabela de redes, gráfico de ocupação por
   canal e recomendação.

## Extra — Saúde da rede e do computador ✅ (2026-10-29)

Tela **Saúde** (2º item do menu) e faixa na Visão geral: nota de 0 a 100, "o que fazer" do mais ao
menos importante e cada ponto com a prova (o número que levou à conclusão). Rede: estabilidade,
quedas do Wi-Fi do PC, tempo de resposta, perda, horário de pico, velocidade × plano, **fila cheia
(bufferbloat)** medida durante o teste de velocidade, DNS, sinal, canal, 2,4 GHz lotado, aparelhos,
proteção do Wi-Fi. Computador: memória e pressão de memória, processador, disco, temperatura, tempo
ligado (lidos de /proc e /sys a cada minuto; não pesa). Sem nota enquanto o básico não tiver dados.
Plano contratado em Configurações (o técnico disse 350/150 Mbps no cabo).

Primeiro achado real: com a internet ocupada a resposta foi de 89 para 391 ms (nota D).

## Ideias para depois (dono disse que não é necessário agora)

- Alertas fora do painel (Telegram/Discord/e-mail).
- Backup e restauração do banco.
- Tema escuro no estilo de um painel financeiro que o dono mostrou (fundo azul-esverdeado escuro,
  destaque verde-água, títulos pequenos em maiúsculas) com botão claro/escuro: só cores e acabamento,
  as telas já usam variáveis.
- Guarda menor dos detalhes no Pi (cartão de memória).

## Fase 4 — DNS e anúncios (depende do Pi)

- Cliente Pi-hole v6 (sessão, renovação, falhas).
- Tela DNS e anúncios: resumo, top domínios, consultas recentes, listas, regras permitir/bloquear,
  pausar/reativar com confirmação. Texto explicando o limite do bloqueio por DNS.
- Guia (só texto) de como apontar o DNS da casa para o Pi-hole — o NetGuard não muda o roteador.
- Alerta "Pi-hole fora".

**Aceite:** com o Pi-hole falso dos testes: fora do ar, senha errada e sessão vencida mostram erro
claro sem derrubar o resto; ações pedem confirmação e entram na auditoria; com o Pi-hole real,
números batem com o painel dele.

## Fase 5 — Consumo e alertas

- Tráfego da interface (taxa, histórico, acumulado por dia/mês).
- SNMP do roteador **se** existir; senão, tela explica o que falta para cada visão.
- Regras de alerta pela tela (latência, perda, aumento anormal de falhas), silêncio entre
  repetições, histórico de alertas.
- Primeiro canal externo (a decidir: Discord, Telegram ou e-mail).

**Aceite:** tráfego da interface bate com `ip -s link` no mesmo intervalo; virada de contador não
gera pico falso; nenhuma tela mostra consumo por aparelho sem fonte real; alerta repetido respeita
o silêncio; falha do canal externo não perde o alerta do painel.

## Fase 6 — Estabilização

- Suíte inteira, revisão de segurança, medição de memória/CPU em uso real (meta: < 50 MB RSS, CPU
  quase zero entre ciclos).
- `scripts/backup.sh` e `restaurar.sh` testados de verdade.
- Disco: guarda dos detalhes configurável (hoje 35 dias ≈ 650 MB; no Pi com cartão, ~14 dias) e,
  se preciso, resumos por hora no lugar do bruto.
- README completo: instalação no Arch, operação, atualização, solução de problemas, migração para
  o Pi (Docker normal, `network_mode: host`, postgres próprio ajustado).

**Aceite:** backup restaurado num database limpo volta com tudo; instalação do zero seguindo o
README funciona; nenhuma falha alta/média na revisão.
