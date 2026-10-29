# Histórico

- 2026-10-29 — Projeto criado só com documentação, a partir do pedido do dono
  ([ESPECIFICACAO.md](ESPECIFICACAO.md)). Análise da máquina e dos riscos, arquitetura, dependências,
  roadmap e decisões em aberto. Nada instalado, sem código, sem git.
- 2026-10-29 (2) — Medição à mão só com este PC, com ok do dono ([MEDICOES.md](MEDICOES.md)):
  varredura da /24 achou 8 aparelhos + o PC (passivo só via 2), nomes por DNS reverso no roteador
  Intelbras, 4 MACs aleatórios, UPnP desligado, mDNS sem resposta. Internet ~242/85 Mbit/s,
  latência ~41–49 ms, 0% de perda. Roadmap ganhou teste de velocidade e qualidade do Wi-Fi; Pi ainda
  vai ser comprado.
- 2026-10-29 (3) — **Fase 1 pronta.** Go 1.27.3 instalado pelo dono; usuário `netguard` +
  databases `netguard`/`netguard_test` no postgres da infra (só o container do postgres foi ligado).
  Backend: config por ambiente, migrações embutidas com trava, login argon2id + sessões (só hash do
  token no banco) + CSRF + limite de 5 tentativas/min + auditoria, só rede local, cabeçalhos de
  segurança, monitor de interface e gateway (ping sem root) a cada 10 s gravando em `verificacoes`,
  `/saude`, `/visao-geral`, frontend embutido, encerramento limpo. Frontend: login, menu com as 8
  áreas (responsivo), Visão geral com dados reais e "indisponível" no resto. Testes: Go 5 pacotes
  ok (banco e ping reais), Vitest 16 ok, tipos ok. Teste de fumaça com o binário: gateway 7,0 ms,
  0% de perda. Memória 38 MB parado. TypeScript fixado na 6. Serviço systemd escrito, não instalado.
- 2026-10-29 (4) — **Fase 2, parte A.** Monitor mede a cada 10 s: roteador, 1.1.1.1/8.8.8.8/9.9.9.9
  (TCP 443 se o ping for bloqueado), DNS configurado (8.8.8.8) × 1.1.1.1, google e cloudflare por
  HTTPS. Diagnóstico com causa provável (12 cenários testados), quedas com histerese, cobertura do
  monitor, migração 0002. Telas: Visão geral com gráfico da última hora e disponibilidade; Internet
  com períodos 1 h a 30 dias, latência, perda, faixa vermelha nas quedas, cada verificação e lista
  de quedas; Histórico de eventos. uPlot 1.6.32. Testes: Go 7 pacotes (com -race), Vitest 22.
  Conferido em navegador real (prints claro/escuro, celular sem rolagem lateral).
- 2026-10-29 (5) — **Fase 2, parte B — fase 2 completa.** Teste de velocidade (medido de verdade:
  273 ↓ / 104 ↑ Mbps, servidor GIG, 2,4 s, 35 MB), agendado 3×/dia + botão com confirmação;
  Wi-Fi a cada minuto pelo nmcli (sinal 79%, canal 48, 1170 Mbps de ligação); tela de
  Configurações que vale na hora. Migração 0003. Corrigido: Wi-Fi lido antes da primeira medição
  aparecia como "sem Wi-Fi"; barra do celular esticava em página curta. Testes: Go 10 pacotes
  (com -race), Vitest 28; prints conferidos no claro, escuro e celular.
- 2026-10-29 (6) — **Fase 3 — aparelhos.** Vizinhos com estado (`ip -j neigh`), procura ativa
  com confirmação, nome pelo roteador, fabricante (OUI), MAC aleatório, aparições, apelido/notas/
  conhecido, aviso de aparelho novo, aparelhos novos no Histórico, card na Visão geral. Teste real:
  9 aparelhos achados; corrigido aviso falso de "novo" ao ligar a procura (primeira procura =
  retrato). Conversado: geolocalização (não dá, e não faz sentido) e Wi-Fi da vizinhança (dá, mas o
  NetworkManager bloqueia a busca para o serviço; `scripts/testar-busca-wifi.sh` para testar).
  Documentação revisada inteira: `USO.md` (guia de cada tela), `ARQUITETURA.md` reescrita como está
  hoje. Testes: Go 83 (11 pacotes, -race), Vitest 33.
- 2026-10-29 (7) — **Wi-Fi da vizinhança e melhor canal.** Busca era recusada ("not authorized"):
  a tela do PC fica no login e o dono acessa remoto. Regra do polkit (só `wifi.scan`, só o usuário do serviço)
  criada pelo dono → 14 a 23 redes vistas. Análise de canal com o porquê (pedido do dono: "canal com
  mais gente pode ser melhor"), pesando o sinal; média das últimas 6 buscas depois de ver a
  recomendação do 2,4 GHz mudar entre duas buscas seguidas. 5 GHz no 48: ótimo. Testes: Go 93, Vitest 38.
- 2026-10-29 (8) — Vizinhos na comparação de canais um por linha, com barrinha de força (pedido do
  dono). Consumo medido no serviço em uso: <1% de CPU, 28 MB de memória; banco 9 MB hoje, ~19 MB/dia
  até ~650 MB com a guarda de 35 dias.
- 2026-10-29 (9) — **Saúde da rede e do computador.** Tela Saúde com nota, o que fazer e a prova de
  cada ponto (13 da rede, 5 do computador). Nova medição: resposta durante o teste de velocidade
  (fila cheia); no teste real foi de 89 para 391 ms (nota D) e o upload deu 45% do plano. Computador
  por /proc e /sys (memória, PSI, carga, disco, temperatura 56–70 °C). Plano 350/150 nos ajustes.
  Corrigido: nota aparecia com quase nada medido; números com ponto. Migração 0005. Dono decidiu: sem
  alertas, backup ou tema novo por enquanto (anotados como ideias). Testes: Go 116, Vitest 43.
- 2026-10-29 (10) — Dono mostrou o painel em uso (modo escuro do sistema; tema novo não precisa).
  No print, faixas vermelhas às 11:02 e 11:18 e 99,60% sem queda: eram ciclos "fora · DNS" durante a
  busca de redes Wi-Fi do próprio NetGuard. Corrigido: esses ciclos são descartados. Os de 10:54–10:55
  e 11:30 foram testes de fumaça meus rodando ao mesmo tempo na mesma rede (não são da internet).
- 2026-10-29 (11) — Aparelho "novo" Gaoshengda = TV TCL (10.0.0.156). Pedido do dono: ações
  visuais. Procura com radar, barra e aparelhos surgindo ao vivo; card "Entrou e saiu"; teste de
  velocidade com fases e velocidade ao vivo; busca de Wi-Fi com barra. Plano do dono: desligar todos e
  religar um por vez para dar nome. Procura real: 11 aparelhos (inclui um iPhone 10.0.0.57).
- 2026-10-29 (12) — Dono tirou um celular do Wi-Fi e ele seguia "na rede" (janela de 10 min). O
  NetGuard já sabia (último visto 11:54, não respondeu à procura de 11:55): a tela agora mostra
  amarelo "não respondeu à última procura" na hora; "saiu" continua após 10 min.
- 2026-10-29 (13) — Resumo de presença: na lista "14× em 7 dias · 31 h"; na ficha entradas, tempo,
  média por vez e hora em que costuma chegar. Dono já nomeou "Roteador da sala", "Celular do
  trabalho" (10.0.0.175) e "TV do quarto" (10.0.0.156).
- 2026-10-29 (14) — Dono: "no canal tinha muito mais gente" e "não vejo o sinal dos vizinhos". Causa:
  cada busca ouvia de 8 a 32 redes e a tela mostrava a última (12:12 = 8). Agora: duas escutas por
  busca (34–37 redes), lista "Redes por perto" juntando as últimas buscas (44), sempre aberta, com
  sinal, faixa, canal e "vista em X de N".
- 2026-10-29 (15) — Dono: "a latência desapareceu e os dispositivos estão duplicados". Causa: ao sair da
  sessão (acesso remoto) às 12:47 o systemd derrubou os serviços do usuário (NetGuard e o Docker do
  postgres) — `Linger=no`; voltou às 14:00 no novo login. Gráficos ficaram sem dado nesse tempo e a
  lista "Entrou e saiu" mostrou todo mundo saindo e entrando de novo. Agora a parada do NetGuard (buraco
  de mais de 2 min entre execuções) vira uma linha "◌ parado" e esconde as saídas/entradas que ela
  causou (também no "N× em 7 dias"). Menu avisa e oferece `loginctl enable-linger`.
