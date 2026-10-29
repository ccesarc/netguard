# Guia de uso

Como usar o NetGuard no dia a dia e o que cada número quer dizer. Para instalar e rodar, ver o
[README](../README.md).

## Abrir o painel

- Neste PC: `http://127.0.0.1:8095`
- De outro aparelho da casa (celular, notebook): `http://10.0.0.209:8095`
  (precisa da regra do firewall: `scripts/liberar-firewall.sh`, uma vez)
- Usuário e senha: os criados com `scripts/primeira-vez.sh` (ou `bin/netguardd usuario criar`).
  A sessão dura 7 dias e se renova sozinha enquanto você usa.

Pelo terminal: `painel` → **NetGuard** → **Abrir o menu do próprio projeto** (ou
`~/projects/netguard/scripts/menu.sh`).

## Visão geral

O resumo da rede agora. Atualiza a cada 10 segundos.

| Card | O que mostra |
|---|---|
| **Internet** | Se a internet está **No ar**, **Com problema** ou **Fora**, a latência média até fora de casa e a causa provável. |
| **Roteador (gateway)** | Se o roteador responde e em quanto tempo. Até ~10 ms pelo Wi-Fi é normal. |
| **DNS** | O DNS que o PC usa (hoje 8.8.8.8) e um DNS de comparação (1.1.1.1). |
| **Este aparelho** | Placa de rede, endereço e o Wi-Fi (rede e força do sinal). |
| **Última hora — latência** | Gráfico do roteador, da internet e do DNS. Passe o mouse para ver cada minuto. Faixa vermelha = internet fora. |
| **Disponibilidade** | Quanto das últimas 24 h a internet esteve no ar. |
| **Velocidade** | O último teste de velocidade. |
| **Dispositivos** | Quantos aparelhos estão na rede agora e se apareceu algum novo. |

Quando a internet cai, aparece uma faixa vermelha no topo com a causa e há quanto tempo.

## Saúde

A leitura pronta: **nota de 0 a 100**, **o que fazer** (do mais ao menos importante) e cada ponto
da rede e do computador com um selo (✔ bom, ➜ dica, ! atenção, ✖ problema, … sem dados), o resumo
e, em letra menor, **a prova** (o número que levou à conclusão).

- **Fila cheia:** mede a resposta da internet *enquanto* o teste de velocidade baixa e envia. Se
  piora muito, chamadas e jogos travam quando alguém baixa ou assiste vídeo. Resolve-se com "QoS",
  "SQM" ou "controle de banda inteligente" no roteador, se ele tiver.
- **Velocidade × plano:** informe o plano em Configurações (aqui: 350/150). Pelo Wi-Fi, 60% ou mais
  é normal; no cabo, 85% ou mais.
- **Horário de pico:** precisa de 1 a 2 dias medindo para comparar as horas do dia.
- **Computador:** memória (e se programas estão esperando por ela), processador, disco,
  temperatura e tempo ligado, lidos de arquivos do sistema. Não pesa no PC.
- Sem nota enquanto o básico (quedas e tempo de resposta) não tiver dados: melhor que um número
  enganoso.

A faixa no alto da Visão geral mostra a nota e a próxima melhoria.

## Internet

Escolha o período no alto (1 hora até 30 dias).

- **Disponibilidade 24 h / 7 dias / 30 dias.** "No ar" conta o tempo em que a internet funcionou
  (lenta também conta). O tempo com o PC desligado **não conta** nem como queda nem como no ar:
  aparece em "medido em X% do período". Ex.: "100,00% · medido em 40% do período" = nas horas em que
  o NetGuard estava ligado, a internet não caiu.
- **Velocidade da internet.** Último teste, histórico e quanto os testes gastaram de dados.
  **Testar agora** pede confirmação (gasta ~35 MB e deixa a internet lenta por alguns segundos) e
  mostra ao vivo a fase (resposta → download → upload), os MB e a velocidade subindo.
  Mede até este PC: pelo Wi-Fi, o número inclui o Wi-Fi.
- **Wi-Fi deste aparelho.** Força do sinal (acima de 70% é ótimo; abaixo de 30% é fraco), faixa
  (2,4 ou 5 GHz), canal, a velocidade da ligação com o roteador (não é a da internet) e o gráfico do
  sinal. "Pior momento" mostra a queda mais funda de cada intervalo.
- **Latência.** Gráfico do período. Cada ponto é a média de um intervalo (1 min na visão de 1 hora,
  6 h na de 30 dias).
- **Perda de pacotes.** Quanto se perdeu no caminho. Perto de 0% é o normal.
- **Quedas.** Cada queda com início, fim, duração e causa.
- **Agora — cada verificação.** O resultado de cada teste do último ciclo.

### Wi-Fi da vizinhança e melhor canal

Mostra as redes Wi-Fi por perto (nome, canal e sinal, que todo roteador anuncia) e qual canal está
mais livre, para cada faixa.

- **Barras:** cada coluna é um canal; a altura é o sinal do vizinho mais forte nele. O **teu canal**
  fica destacado em laranja. No 5 GHz, os canais esmaecidos são **DFS** (dividem espaço com radar;
  o roteador pode pular de canal sozinho), por isso não são recomendados.
- **Comparando os canais:** do melhor para o pior, com o motivo. Embaixo de cada canal, um vizinho
  por linha com a barrinha da força (vermelha = forte, 60%+; laranja = média, 40–59%; verde = fraca)
  e, no 2,4 GHz, os que estão em canais que se sobrepõem. A conta pesa **o quão forte** cada vizinho
  chega, não só quantos são: um canal com três redes médias pode ser melhor que um com uma só forte.
- **Recomendação:** usa a média das últimas buscas (a vizinhança muda de uma busca para outra) e
  só diz "vale trocar" quando a melhora é clara. Trocar o canal é **no painel do roteador**.
- **Limite:** dá para ver o quão alto cada vizinho "fala", não o quanto ele usa a internet.
- **Redes por perto:** sempre aberta, uma por linha, juntando as últimas buscas (~3 h): barrinha e
  % do sinal (média das vezes em que foi ouvida), faixa, canal e "3/6" = ouvida em 3 das últimas 6
  buscas. Cada busca só ouve parte das redes (fraca ou longe às vezes escapa), por isso a lista junta
  várias e cada busca escuta duas vezes seguidas.
- A busca roda a cada 30 min e pausa o Wi-Fi deste PC por ~1 s. **Buscar agora** faz na hora.

### As causas, em português

| Causa | Quer dizer | O que fazer |
|---|---|---|
| Rede local: este aparelho está sem rede | O Wi-Fi ou o cabo **deste PC** caiu. A internet da casa pode estar normal. | Ver o Wi-Fi do PC (o `wifi-watchdog` costuma resolver sozinho). |
| Roteador não responde | O PC está na rede, mas o roteador não responde. | Ver se o roteador está ligado; reiniciar. |
| Sem internet (provedor ou roteador) | O roteador responde, mas nada de fora. | Geralmente provedor; às vezes o roteador perdeu a conexão. Reiniciar o roteador; se continuar, ligar para o provedor. |
| DNS configurado não responde | A internet funciona, mas o DNS do PC (8.8.8.8) não. | Trocar o DNS do PC, ou esperar. |
| DNS não responde | Nenhum DNS testado responde. | Parecido com "sem internet". |
| Um serviço específico está fora | Um dos sites testados não abriu, mas o resto funciona. | Problema do site, não da tua internet. |
| Internet lenta / Perdendo pacotes | Funciona, mas devagar ou falhando às vezes. | Comum em horário de pico. Se for sempre, falar com o provedor (com os gráficos na mão). |

Uma queda só é registrada depois de **3 verificações seguidas** sem internet (~30 s), para um
pacote perdido não virar queda. Ela fecha depois de 2 verificações boas.

## Dispositivos

Os aparelhos que o NetGuard **observou** na rede de casa.

- **● verde** = respondeu na última procura. **◐ amarelo "não respondeu"** = não respondeu à
  última procura (ou some há mais de 6 min): provavelmente saiu, ou o celular está dormindo.
  **○ vazia** = fora da rede há mais de 10 minutos (é quando entra como "saiu" na lista).
- **Nome:** o apelido que você deu; senão o nome que o roteador conhece (ex.: "Celular-Android");
  senão o fabricante.
- **MAC aleatório (privacidade):** celulares, iPads e Macs inventam um endereço por rede para não
  serem rastreados. Por isso não dá para saber o fabricante deles. O nome e o apelido resolvem.
- **Procurar agora:** manda um ping para cada endereço da rede (10.0.0.1 a 10.0.0.254), uma vez.
  Enquanto procura, mostra um radar e a barra ("120 de 253 endereços · 7 aparelhos responderam") e
  os aparelhos vão surgindo na lista, em verde com "acabou de aparecer". No fim: "Procura concluída
  em 18 s: 11 na rede · 2 apareceram agora".
- **Entrou e saiu (últimas 24 h):** cada entrada e saída com a hora; clicar no nome abre a ficha.
  "Saiu" só aparece depois de 10 min sem responder (celular com a tela apagada some e volta).
  "◌ parado" = o NetGuard ficou desligado nesse tempo; quem estava antes e respondeu logo depois não
  aparece como saída e entrada.
- **Para dar nome a todos:** desligue o Wi-Fi de um aparelho, religue, clique em "Procurar agora" e
  veja quem "entrou"; abra e dê o apelido. No celular, desligue e ligue o Wi-Fi, mas **não use
  "Esquecer rede"** (com MAC aleatório ele pode voltar como outro aparelho).
- **Procura automática** (desligada de fábrica): faz isso a cada 5 minutos. Sem ela, só aparecem
  os aparelhos que conversaram com este PC. Liga no botão **Ligar procura** ou em Configurações.
- **Aparelho novo:** quando surge um aparelho que nunca esteve na rede, aparece um aviso amarelo:
  - **É conhecido** marca como confiável e some o aviso;
  - **Já vi** só tira o aviso;
  - **Detalhes** abre a ficha.

  A primeira procura de todas é o "retrato da casa": nada vira aviso.
- **Na lista**, cada aparelho mostra quantas vezes entrou e quanto tempo ficou nos últimos 7 dias
  (ex.: "14× em 7 dias · 31 h").
- **Ficha do aparelho** (clique na linha): IP, MAC, fabricante, nome na rede, primeira vez visto,
  apelido, notas, "conhecido", o resumo dos **últimos 7 dias** (quantas vezes entrou, tempo na rede,
  média por vez, hora em que costuma chegar, com 3+ entradas) e **quando esteve na rede** (cada
  período de presença). Sair e voltar em menos de 10 min conta como a mesma visita. O histórico
  fica guardado por 400 dias.

O NetGuard **não vê o que os aparelhos acessam** nem quanto cada um consome: só que estão na
rede. A lista pode não estar completa: aparelho dormindo ou numa rede de visitantes isolada pode
não aparecer.

## Histórico de eventos

Quedas da internet e aparelhos que apareceram pela primeira vez, nos últimos 30 dias.

## Configurações

Tudo aqui passa a valer **na hora**, sem reiniciar, e fica registrado (quem mudou, quando, o quê).
O botão **Salvar** mostra o que vai mudar e pede confirmação. **Valores de fábrica** preenche o
formulário com o padrão (ainda precisa salvar).

| Campo | Padrão | Para que serve |
|---|---|---|
| Intervalo entre verificações | 10 s | De quanto em quanto tempo mede. Menos = queda detectada mais rápido. |
| Endereços de fora para o ping | 1.1.1.1, 8.8.8.8, 9.9.9.9 | Os "pontos de fora" da internet. Precisam ser da internet, não da rede de casa. |
| DNS de comparação | 1.1.1.1 | Para saber se a culpa é só do DNS configurado. |
| Sites testados por HTTPS | Google e Cloudflare | Sites para conferir se abrem. Pode deixar vazio. |
| Latência acima de | 150 ms | Daí para cima conta como "lenta". |
| Perda de pacotes acima de | 10% | Daí para cima conta como "perdendo pacotes". |
| Teste de velocidade | 3×/dia, 25 MB ↓ + 10 MB ↑ | Gasto estimado aparece embaixo (~3,1 GB/mês no padrão). |
| Procura de aparelhos | desligada | Ver "Dispositivos". |
| Plano contratado | 0 (não sei) | Download e upload do plano, no cabo. Aqui: 350 e 150. Usado na Saúde. |

## Perguntas frequentes

**O PC estava desligado. A internet "caiu"?**
Não. O tempo desligado fica como "sem dados" e não entra na conta.

**Por que o teste de velocidade dá menos que o plano contratado?**
Ele mede até este PC, pelo Wi-Fi. Pelo cabo, ou no Raspberry Pi no cabo, o número fica mais
perto do plano.

**Dá para saber a localização dos aparelhos?**
Não. O NetGuard só sabe se o aparelho está **na rede de casa** e quando entrou e saiu. Fora de
casa ele some da rede. A casa em si só é localizável na escala da cidade (pelo IP público), e o
NetGuard não usa isso.

**Dá para ver o Wi-Fi dos vizinhos e o melhor canal?**
Sim: tela **Internet → Wi-Fi da vizinhança e melhor canal**. Precisa da regra criada por
`scripts/permitir-busca-wifi.sh` (já feita neste PC); sem ela o sistema recusa a busca quando
ninguém está logado na tela do PC.

**O NetGuard pesa no computador?**
Não. Medido em 2026-10-29: menos de 1% de um núcleo do processador e ~28 MB de memória. O banco
cresce ~19 MB por dia e para em ~650 MB (os detalhes de cada verificação ficam 35 dias; os resumos,
quedas e testes ficam mais). No Raspberry Pi vale guardar menos (ver ROADMAP, fase 6).

**Quanto de internet o NetGuard gasta?**
Os testes de velocidade: ~35 MB cada (~3,1 GB/mês no padrão; ajustável). O resto (pings, DNS,
sites) é pouco: menos de 1 MB por hora.

**Alguém de fora consegue abrir o painel?**
Não. O NetGuard recusa qualquer acesso que não venha da rede de casa, e o firewall só libera a
porta 8095 para 10.0.0.x. Também não há porta aberta no roteador.

**Esqueci a senha.**
No Konsole: `painel` → NetGuard → menu → **7 (Usuários)** → **Trocar senha**.
