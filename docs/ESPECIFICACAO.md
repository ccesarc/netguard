# Especificação original

Pedido do dono em 2026-10-29, guardado como fonte dos requisitos (texto original; só as listas
das seções 8 e 9 foram juntadas em parágrafos). As adaptações
(o que muda por causa da máquina, da rede ou do que já existe) estão em [ANALISE.md](ANALISE.md)
e [DECISOES.md](DECISOES.md).

---

## 1. Objetivo

Desenvolva um aplicativo web completo para monitorar, diagnosticar e proteger uma rede doméstica, com foco em disponibilidade da internet, dispositivos conectados, desempenho, segurança e bloqueio de anúncios.

O sistema deverá funcionar inicialmente em um computador com Arch Linux e, futuramente, poderá ser instalado em um Raspberry Pi.

Quero uma solução funcional, modular, segura, de baixo consumo de recursos e com interface moderna.

Antes de implementar, analise a arquitetura proposta, identifique riscos técnicos e apresente um plano de execução por etapas. Não faça alterações destrutivas sem autorização.

## 2. Stack tecnológica

- Frontend: Vue 3, TypeScript, Vite e Composition API.
- Backend: Go, com API REST.
- Comunicação em tempo real: WebSocket, quando necessário.
- Banco de dados: PostgreSQL para histórico e configurações persistentes.
- Bloqueio de anúncios: integração com Pi-hole por sua API oficial.
- Infraestrutura: Docker Compose, com possibilidade de execução nativa no Linux.
- Testes: Vitest para frontend e testes automatizados para o backend.
- Documentação: README com instalação, configuração, operação e solução de problemas.

Não utilize React ou Nuxt. Evite dependências desnecessárias e mantenha o consumo de memória baixo.

## 3. Funcionalidades

### 3.1. Dashboard

Criar um painel inicial com:

- Estado da internet e da rede local.
- Latência atual e histórico.
- Perda de pacotes.
- Disponibilidade nas últimas 24 horas, 7 dias e 30 dias.
- Quantidade de dispositivos detectados.
- Eventos de queda e recuperação.
- Consultas DNS e domínios bloqueados, quando disponíveis.
- Alertas recentes.
- Gráficos históricos com atualização automática.

Os indicadores devem utilizar dados reais. Nunca preencher o painel com valores fictícios apresentados como se fossem medições verdadeiras.

### 3.2. Monitoramento da internet

Implementar verificações periódicas para:

- Gateway local.
- Conectividade com endereços externos.
- Resolução DNS.
- Disponibilidade de serviços externos por HTTP/HTTPS.
- Latência, perda de pacotes e tempo de recuperação.

Permitir configurar intervalos, destinos de teste e limites de alerta.

Registrar início e fim das interrupções, duração, horário e resultados das verificações.

Distinguir, quando possível:

- Falha na rede local.
- Gateway indisponível.
- Falha de conectividade externa.
- Problemas de DNS.
- Indisponibilidade de um serviço específico.

Não classificar automaticamente toda falha como problema da operadora. Utilizar os resultados das verificações para determinar a causa provável.

### 3.3. Descoberta de dispositivos

Implementar descoberta de dispositivos na rede local utilizando mecanismos compatíveis com Linux, como ARP, tabelas de vizinhos e varreduras autorizadas.

Exibir:

- Endereço IP.
- Endereço MAC, quando disponível.
- Nome do dispositivo, quando descoberto.
- Fabricante, quando identificável.
- Última vez observado.
- Estado estimado: ativo ou não observado.
- Histórico de aparições.
- Identificação manual e apelidos personalizados.

Permitir marcar dispositivos como confiáveis e gerar alertas para novos dispositivos detectados.

Não afirmar que a lista contém todos os aparelhos conectados quando a topologia ou as permissões da rede impedirem a descoberta.

### 3.4. Monitoramento de consumo de banda

Criar uma área para acompanhar o tráfego de rede.

Exibir taxas de download e upload, gráficos históricos e consumo acumulado quando os dados estiverem disponíveis.

Diferenciar claramente:

- Tráfego total da interface do servidor.
- Tráfego agregado da rede.
- Tráfego individual por dispositivo.

Não estimar o consumo individual a partir do tráfego total. Para métricas por dispositivo, avaliar integrações com SNMP, contadores do roteador, NetFlow/IPFIX ou outros mecanismos compatíveis.

Informar quais recursos dependem do modelo do roteador, de permissões administrativas ou de equipamentos adicionais.

### 3.5. Bloqueio de anúncios e rastreadores

Integrar o Pi-hole utilizando sua API oficial.

Permitir:

- Consultar o estado do serviço.
- Exibir total de consultas DNS.
- Exibir consultas bloqueadas.
- Consultar os domínios mais solicitados e bloqueados.
- Gerenciar listas de bloqueio por meio dos recursos oficiais disponíveis.
- Consultar registros DNS e estatísticas.
- Criar regras personalizadas de permissão e bloqueio.
- Pausar ou reativar o bloqueio, com confirmação.
- Exibir o histórico de consultas quando suportado pela integração.

Explicar que o bloqueio por DNS não elimina todos os anúncios, especialmente aqueles servidos pelo mesmo domínio do conteúdo, como ocorre em determinados serviços de vídeo.

O aplicativo não deve modificar automaticamente o DNS do roteador. A configuração para toda a rede deve ser orientada e autorizada pelo usuário.

### 3.6. Alertas

Implementar um sistema de alertas para:

- Queda da internet.
- Recuperação da conexão.
- Latência acima do limite configurado.
- Perda excessiva de pacotes.
- Novo dispositivo detectado.
- Pi-hole indisponível.
- Aumento anormal de falhas.

Criar uma interface de configuração dos limites e dos canais de notificação.

Implementar primeiro alertas locais no painel. Preparar uma arquitetura extensível para notificações externas.

### 3.7. Segurança

- Não armazenar senhas em texto puro.
- Proteger credenciais de APIs e tokens.
- Não expor a API de administração diretamente à internet.
- Implementar autenticação para operações administrativas.
- Validar todos os parâmetros recebidos pela API.
- Aplicar limites de requisições quando necessário.
- Registrar ações administrativas relevantes.
- Não executar comandos arbitrários fornecidos pelo frontend.
- Utilizar privilégios mínimos no Linux.
- Solicitar autorização antes de realizar varreduras ou alterações na rede.
- Não capturar conteúdo privado das comunicações.
- Evitar que a interface permita desativar a proteção ou alterar regras sem confirmação.

## 4. Interface

Criar uma interface moderna, responsiva e em português brasileiro.

Menu lateral:

1. Visão geral.
2. Internet.
3. Dispositivos.
4. Consumo de rede.
5. DNS e anúncios.
6. Alertas.
7. Histórico de eventos.
8. Configurações.

Utilizar componentes reutilizáveis, gráficos legíveis, estados de carregamento, mensagens de erro e indicadores claros de dados indisponíveis.

Não utilizar informações simuladas no ambiente de produção.

## 5. Arquitetura e organização

Separar o projeto em módulos:

- API e autenticação.
- Monitor de conectividade.
- Descoberta de dispositivos.
- Coleta de métricas.
- Integração com Pi-hole.
- Histórico e persistência.
- Alertas.
- Frontend.

O backend deve executar verificações em segundo plano sem depender de uma página aberta no navegador.

Implementar timeouts, cancelamento de operações, tratamento de erros, logs estruturados e recuperação de falhas.

Criar migrações do banco de dados e utilizar consultas parametrizadas.

## 6. Instalação

O sistema deverá oferecer:

- Instalação documentada no Arch Linux.
- Configuração por arquivo de ambiente.
- Docker Compose para facilitar a instalação.
- Persistência dos dados após reinicialização.
- Inicialização automática dos serviços.
- Verificação de saúde dos componentes.
- Procedimentos de backup e restauração.
- Orientações para migração futura para Raspberry Pi.

Não presumir que o usuário possui acesso administrativo ao roteador.

O modo inicial deverá funcionar com um computador conectado à rede local, sem exigir mudanças no roteador.

## 7. Testes e critérios de aceite

Criar testes automatizados para:

- Falha e recuperação da conexão.
- Cálculo da disponibilidade.
- Medição de latência e perda de pacotes.
- Descoberta e atualização de dispositivos.
- Persistência do histórico.
- Falha na comunicação com o Pi-hole.
- Expiração ou ausência de credenciais.
- Erros de DNS.
- Reconexão após reinicialização.
- Autenticação e autorização.
- Tratamento de dados indisponíveis.
- Responsividade do frontend.

Os testes devem ser reproduzíveis e não depender de interromper a internet doméstica real.

## 8. Estratégia de desenvolvimento

**Fase 1 — Fundação:** analisar requisitos e arquitetura; criar a estrutura do projeto; configurar frontend, backend, banco e testes; implementar o dashboard com dados reais.

**Fase 2 — Monitoramento:** verificações do gateway, internet e DNS; histórico e disponibilidade; gráficos e eventos.

**Fase 3 — Dispositivos:** descoberta e histórico; identificação manual e alertas de novos dispositivos.

**Fase 4 — Bloqueio de anúncios:** integrar o Pi-hole; estatísticas, regras e controles administrativos.

**Fase 5 — Consumo e alertas:** métricas compatíveis com a infraestrutura; notificações e histórico de alertas.

**Fase 6 — Estabilização:** testes de regressão; segurança e consumo de recursos; documentar instalação, atualização e recuperação.

## 9. Regras de execução

Antes de escrever código, apresente: arquitetura proposta, estrutura de diretórios, dependências necessárias, riscos e limitações técnicas, plano de implementação e critérios de aceite.

Depois, implemente uma fase por vez. Ao concluir cada fase, execute os testes e apresente um resumo das alterações, arquivos modificados, resultados dos testes, riscos pendentes e próximos passos.

Preserve as funcionalidades existentes caso o projeto seja incorporado a um repositório já existente. Não apague arquivos, substitua configurações ou faça mudanças destrutivas sem autorização.

Priorize funcionamento real, segurança, simplicidade de manutenção e baixo consumo de recursos, em vez de criar apenas uma interface visual impressionante.

Comece pela análise e pelo plano técnico. Não implemente todas as fases de uma só vez.
