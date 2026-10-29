# Decisões

## Em aberto (precisa do dono)

| # | Pergunta | Muda o quê |
|---|---|---|
| D1 | Raspberry Pi: **ainda vai comprar** (2026-10-29). Qual modelo e quanta memória? | quando o monitor sai deste PC; Pi-hole; postgres ou não no Pi (D5) |
| D2 | Roteador **Intelbras W6-1500**, painel em `http://10.0.0.1/`, UPnP desligado. **Dono não tem a senha** (2026-10-29). Sistema segue sem depender do roteador; recuperar a senha (provedor/etiqueta) fica opcional | lista de clientes/consumo, SNMP, DNS para o Pi-hole |
| D3 | Pi-hole: **não existe ainda** (é programa gratuito; vai no Pi quando chegar) | fase 4 (só v6 é suportada) |
| D4 | O monitor pode ficar no cabo? | confiabilidade do diagnóstico (Wi-Fi do PC trava) |
| D5 | PostgreSQL também no Pi, ou SQLite lá? | o pedido diz PostgreSQL; SQLite gastaria bem menos memória num Pi pequeno |
| D6 | Telas só de leitura também pedem login? Dono perguntou se precisa (só ele usa a rede); recomendação foi manter, sessão longa | seguindo com login em tudo até o dono decidir |
| D7 | Primeiro canal de alerta externo: Discord, Telegram ou e-mail? Dono: **não precisa agora** (2026-10-29) | fase 5 |
| D9 | Repositório no GitHub (`ccesarc/netguard`, privado) ou só local? | push (hoje só git local) |

## Tomadas (proposta, valem até o dono mudar)

| Data | Decisão | Por quê |
|---|---|---|
| 2026-10-29 | `netguardd` roda **nativo** (systemd do usuário); no Docker só o postgres | Docker rootless não vê a rede de casa |
| 2026-10-29 | Um binário só (API + monitor + frontend embutido) | memória e manutenção |
| 2026-10-29 | SSE em vez de WebSocket | painel só recebe; biblioteca padrão, reconecta sozinho |
| 2026-10-29 | Tempo com monitor desligado = "sem dados", fora da conta | não inventar disponibilidade |
| 2026-10-29 | Causa "conectividade externa (provedor ou roteador)", nunca "operadora" | sem acesso ao roteador não dá para separar |
| 2026-10-29 | Varredura ativa desligada por padrão | pedido: autorização antes de varrer |
| 2026-10-29 | Sem consumo por aparelho sem fonte real | pedido: não estimar a partir do total |
| 2026-10-29 | Pi-hole só no Pi, nunca neste PC | PC que desliga derrubaria o DNS da casa |
| 2026-10-29 | Não repetir o Sentinela (sites vistos de fora) | já existe |
| 2026-10-29 | Varredura ativa da rede autorizada pelo dono ("quanto mais informações, melhor"), mantida como opção com confirmação na tela | sem ela só 2 de 9 aparelhos aparecem |
| 2026-10-29 | Fases 1–3 + teste de velocidade + qualidade do Wi-Fi só com este PC, antes do Pi | dono ainda vai comprar o Pi |
| 2026-10-29 | Go e dependências da fase 1 aprovados; Go instalado pelo dono (D8) | fase 1 |
| 2026-10-29 | Usuário próprio `netguard` no postgres da infra, dono só dos databases dele (não o superusuário) | privilégio mínimo |
| 2026-10-29 | TypeScript 6 em vez do 7 | vue-tsc ainda não roda com o 7 |
| 2026-10-29 | Meta de memória: < 50 MB parado (era 30) | medido: 38 MB parado; argon2 usa 19 MiB por login |
| 2026-10-29 | Usuário só pelo terminal (`netguardd usuario criar`), sem cadastro na web | ninguém da rede cria conta |
| 2026-10-29 | DNS "configurado" = servidores reais do systemd-resolved (hoje 8.8.8.8), não o cache 127.0.0.53; comparação com 1.1.1.1 | cache esconderia a falha |
| 2026-10-29 | Queda = 3 ciclos seguidos fora (~30 s); fecha com 2 bons; site fora ou lentidão = "com problema", não queda | pacote perdido não vira queda |
| 2026-10-29 | Gráficos com uPlot; paleta validada para daltonismo (claro e escuro) | leve e feito para séries no tempo |
| 2026-10-29 | Teste de velocidade contra speed.cloudflare.com (sem instalar nada), 25 MB ↓ + 10 MB ↑, 3×/dia ≈ 3,1 GB/mês, configurável | servidor perto (Rio, GIG) e sem conta |
| 2026-10-29 | Wi-Fi pelo `nmcli` com comando fixo, sem `--rescan` | driver `wl` não informa sinal pelo kernel; buscar redes atrapalha o Wi-Fi |
| 2026-10-29 | Configurações no banco (tabela `configuracao`), validadas no servidor; `NETGUARD_INTERVALO` vira só o padrão | mudar sem reiniciar e sem editar arquivo |
| 2026-10-29 | Aparelho "na rede" só com entrada REACHABLE/DELAY/PROBE na tabela de vizinhos (`ip -j neigh`) | o kernel guarda STALE por horas; `/proc/net/arp` não mostra o estado |
| 2026-10-29 | Procura ativa desligada de fábrica, liga na tela com confirmação; a cada 5 min, 16 pings por vez | pedido: autorizar antes de varrer; dono autorizou e liga pelo botão |
| 2026-10-29 | Primeira coleta e primeira procura = retrato da casa, sem aviso de "novo" | senão os 6 aparelhos achados ao ligar a procura viravam aviso falso (visto no teste real) |
| 2026-10-29 | Nome do aparelho pelo DNS reverso do roteador; sem mDNS | o roteador já sabe o nome do DHCP; mDNS é barrado pelo firewall |
| 2026-10-29 | Sem geolocalização | IP local e MAC não dão posição; IP público só dá a cidade; localizar por roteadores vizinhos exigiria mandar dados da rede para Google/Apple |
| 2026-10-29 | Busca de redes liberada por regra do polkit só para a ação `wifi.scan` e só para o usuário do serviço (`/etc/polkit-1/rules.d/50-netguard-busca-wifi.rules`) | dono acessa o PC remoto (tela parada no login) e o serviço roda em segundo plano: os dois eram recusados |
| 2026-10-29 | Disputa de canal = soma de (sinal/100)² dos vizinhos; 2,4 GHz com sobreposição de até 4 canais, só 1/6/11 recomendados; 5 GHz por bloco de 80 MHz, sem recomendar DFS | rede forte atrapalha bem mais que várias fracas; DFS divide com radar |
| 2026-10-29 | Recomendação pela média das últimas 6 buscas; só manda trocar com melhora clara | uma busca só oscila e trocar canal derruba o Wi-Fi de todos por alguns segundos |
| 2026-10-29 | Limite honesto: mede o sinal dos vizinhos, não o quanto eles usam | uso real (tempo no ar) exigiria placa em modo monitor e root |
| 2026-10-29 | Foco do dono: saúde da rede e o que melhorar; alertas, backup e tema novo ficam para depois | pedido do dono |
| 2026-10-29 | Plano contratado: 350 ↓ / 150 ↑ Mbps no cabo (técnico); Wi-Fi entrega "um pouco menos" | comparação na tela Saúde (pelo Wi-Fi, 60%+ é normal; no cabo, 85%+) |
| 2026-10-29 | "Fila cheia" medida com pedidos vazios numa conexão à parte durante o download e o upload do teste | é o que mostra se chamadas/jogos travam quando alguém baixa; nota A+ a F como os medidores conhecidos |
| 2026-10-29 | Saúde do computador só por /proc e /sys (memória, PSI, carga, disco, temperatura); sem SMART | ler arquivos não pesa; SMART exige root |
| 2026-10-29 | Nota da saúde só com estabilidade e tempo de resposta medidos | nota com quase nada medido enganaria (aconteceu no teste: 76 com 7 de 16 sem dados) |
| 2026-10-29 | Ciclo do monitor que coincide com a busca de redes Wi-Fi é descartado (não grava, não conta para queda) | a busca tira a placa do canal por 1–2 s: às 11:02 e 11:18 virou "fora · DNS" e baixou a disponibilidade para 99,60% |
| 2026-10-29 | Cada busca de Wi-Fi escuta duas vezes (3 s entre elas) e a lista "Redes por perto" junta as últimas 6 buscas | uma busca só ouvia de 8 a 32 redes; com duas escutas, 34–37, e 44 juntando duas buscas |
| 2026-10-29 | Porta 8095 (conferida livre) | 8084/8085/8090 já são de outros projetos |
