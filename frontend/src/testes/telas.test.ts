import { afterEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { createMemoryHistory, createRouter } from 'vue-router'
import VisaoGeral from '../views/VisaoGeral.vue'
import Internet from '../views/Internet.vue'
import LayoutPainel from '../layouts/LayoutPainel.vue'
import Indisponivel from '../components/Indisponivel.vue'
import Configuracoes from '../views/Configuracoes.vue'
import Dispositivos from '../views/Dispositivos.vue'
import Saude from '../views/Saude.vue'
import type { RelatorioSaude, Vizinhanca, Ajustes, Dispositivo, Disponibilidades, Historico, ListaDispositivos, Velocidade, VisaoGeral as TVisaoGeral, Wifi } from '../api/tipos'

// O ambiente de teste não tem canvas: o gráfico vira um dublê que mostra o que recebeu.
vi.mock('../components/GraficoSerie.vue', () => ({
  default: defineComponent({
    props: ['t', 'series', 'fora', 'unidade', 'rotulo', 'passoS', 'altura'],
    setup: (p) => () =>
      h('div', { class: 'grafico-dublê', 'data-pontos': p.t.length, 'data-series': p.series.map((s: { nome: string }) => s.nome).join(',') }),
  }),
}))

const indisponivel = (motivo: string) => ({ disponivel: false as const, motivo })
const agoraISO = new Date().toISOString()

const comMedicao: TVisaoGeral = {
  medido_em: agoraISO,
  interface: { disponivel: true, nome: 'wlan0', tipo: 'wifi', ativa: true, ipv4: '10.0.0.209/24', gateway: '10.0.0.1', ok: true },
  gateway: {
    disponivel: true,
    alvo: '10.0.0.1',
    ok: true,
    enviados: 5,
    recebidos: 5,
    perda: 0,
    latencia_min_ms: 2.0,
    latencia_media_ms: 6.84,
    latencia_max_ms: 19.4,
    medido_em: agoraISO,
  },
  internet: {
    disponivel: true,
    estado: 'ok',
    causa: { causa: 'ok', texto: 'Tudo normal' },
    latencia_media_ms: 30.2,
    perda: 0,
    externos: [
      { alvo: '1.1.1.1', ok: true, enviados: 3, recebidos: 3, perda: 0, latencia_min_ms: 19, latencia_media_ms: 20, latencia_max_ms: 21, medido_em: agoraISO },
      { alvo: '8.8.8.8', ok: false, enviados: 3, recebidos: 0, perda: 1, latencia_min_ms: 0, latencia_media_ms: 0, latencia_max_ms: 0, erro: 'sem resposta', medido_em: agoraISO },
    ],
    servicos: [{ alvo: 'https://www.google.com/generate_204', ok: true, ms: 120, status: 204 }],
  },
  dns: { disponivel: true, ok: true, sistema: [{ alvo: '8.8.8.8', ok: true, ms: 31 }], externo: [{ alvo: '1.1.1.1', ok: true, ms: 22 }] },
  queda: null,
  wifi: {
    disponivel: true,
    amostra: { em: agoraISO, ssid: 'MINHA_REDE_5G', sinal: 79, canal: 48, freq_mhz: 5240, taxa_mbps: 1170, redes_mesmo_canal: 0, redes_vistas: 0 },
  },
  velocidade: {
    disponivel: true,
    rodando: false,
    proximo: null,
    ultimo: { em: agoraISO, origem: 'agendado', download_mbps: 242.3, upload_mbps: 85.1, latencia_ms: 48, bytes_baixados: 0, bytes_enviados: 0, duracao_s: 3 },
  },
  dispositivos: indisponivel('fase 3'),
  bloqueio: indisponivel('Precisa do Pi-hole'),
  alertas: indisponivel('fase 5'),
}

const t0 = 1_793_000_000
const historico: Historico = {
  periodo: '1h',
  passo_s: 60,
  t: [t0, t0 + 60, t0 + 120],
  gateway_ms: [7, null, 8],
  gateway_perda: [0, null, 0],
  internet_ms: [30, null, 31],
  internet_perda: [0, null, 0],
  dns_ms: [25, null, 26],
  fora: [0, null, 0],
}

const disponibilidade: Disponibilidades = {
  '24h': { disponivel: true, percentual: 0.9988, cobertura: 0.5, quedas: 1, tempo_fora_s: 40, ciclos: 4320 },
  '7d': { disponivel: true, percentual: 0.9988, cobertura: 0.07, quedas: 1, tempo_fora_s: 40, ciclos: 4320 },
  '30d': indisponivel('Sem medições neste período.'),
}

function respostaJSON(status: number, corpo: unknown) {
  return new Response(JSON.stringify(corpo), { status, headers: { 'Content-Type': 'application/json' } })
}

// api responde por pedaço da URL; o que não estiver no mapa dá 404.
function api(rotas: Record<string, unknown | ((op?: RequestInit) => Response)>) {
  const f = vi.fn(async (url: string, op?: RequestInit) => {
    for (const [trecho, r] of Object.entries(rotas)) {
      if (url.includes(trecho)) return typeof r === 'function' ? (r as (op?: RequestInit) => Response)(op) : respostaJSON(200, r)
    }
    return respostaJSON(404, { erro: 'rota não encontrada' })
  })
  vi.stubGlobal('fetch', f)
  return f
}

const velocidade: Velocidade = {
  estado: { rodando: false, proximo: '2026-10-29T18:00:00Z', ultimo: comMedicao.velocidade.disponivel ? comMedicao.velocidade.ultimo : null, progresso: null },
  testes: [
    { em: '2026-10-29T10:00:00Z', origem: 'agendado', download_mbps: 230, upload_mbps: 80, latencia_ms: 50, bytes_baixados: 0, bytes_enviados: 0, duracao_s: 3 },
    { em: '2026-10-29T12:00:00Z', origem: 'manual', download_mbps: 242.3, upload_mbps: 85.1, latencia_ms: 48, bytes_baixados: 0, bytes_enviados: 0, duracao_s: 3 },
  ],
  gasto_30d_mb: 70,
  estimativa_mes_mb: 3150,
  config: { ativo: true, vezes_por_dia: 3, mb_download: 25, mb_upload: 10 },
}

const wifiResp: Wifi = {
  atual: comMedicao.wifi,
  historico: { passo_s: 900, t: [t0, t0 + 900], sinal: [78, 80], sinal_min: [70, 75], taxa_mbps: [1170, 1170] },
}

const ajustesAtuais: Ajustes = {
  intervalo_s: 10,
  externos: ['1.1.1.1', '8.8.8.8', '9.9.9.9'],
  dns_comparacao: '1.1.1.1',
  urls: ['https://www.google.com/generate_204'],
  limite_latencia_ms: 150,
  limite_perda: 0.1,
  velocidade: { ativo: true, vezes_por_dia: 3, mb_download: 25, mb_upload: 10 },
  varredura_ativa: false,
  plano_download_mbps: 0,
  plano_upload_mbps: 0,
}

const disp = (x: Partial<Dispositivo>): Dispositivo => ({
  id: 1, mac: '', ip: '', nome: '', nome_descoberto: '', apelido: '', notas: '', fabricante: '',
  mac_aleatorio: false, confiavel: false, este_aparelho: false, alertar_novo: false, na_rede: true,
  primeiro_visto: agoraISO, ultimo_visto: agoraISO, ...x,
})

const listaDisp: ListaDispositivos = {
  estado: { ultima_coleta: agoraISO, ultima_varredura: null, ultima_varredura_inicio: null, varrendo: false, varredura_ativa: false, progresso: null },
  resumo: { total: 3, na_rede: 2, novos: 1 },
  dispositivos: [
    disp({ id: 1, nome: 'meuintelbras', ip: '10.0.0.1', mac: '54:6c:ac:12:34:43', fabricante: 'Intelbras', confiavel: true }),
    disp({ id: 2, nome: 'Aparelho sem nome (MAC aleatório)', ip: '10.0.0.28', mac: '66:a6:27:12:34:7f', mac_aleatorio: true, na_rede: false, ultimo_visto: '2026-10-29T08:00:00Z' }),
    disp({ id: 3, nome: 'Aparelho Samsung', ip: '10.0.0.77', mac: '00:11:22:33:44:55', fabricante: 'Samsung', alertar_novo: true, presenca_7d: { entradas: 14, segundos: 31 * 3600 } }),
  ],
}

const vizinhancaResp: Vizinhanca = {
  disponivel: true,
  buscando: false,
  vizinhanca: {
    em: agoraISO,
    redes: [
      { ssid: 'MINHA_REDE', canal: 1, freq_mhz: 2412, sinal: 95, conectada: false, propria: true, seguranca: 'WPA2' },
      { ssid: 'Vizinho1', canal: 6, freq_mhz: 2437, sinal: 90, conectada: false, propria: false, seguranca: 'WPA2' },
      { ssid: 'MINHA_REDE_5G', canal: 48, freq_mhz: 5240, sinal: 79, conectada: true, propria: true, seguranca: 'WPA2' },
    ],
    faixa_24: {
      nome: '2,4 GHz', tua_rede: 'MINHA_REDE', canal_atual: 1, recomendado: 1, mudar: false, vizinhos: 1,
      texto: 'Canal 1 está livre de vizinhos. Não mexa.',
      comparacao: [
        { rotulo: 'canal 1', canal: 1, disputa: 'pouca', pontos: 0.14, motivo: '1 rede: Vizinho2 37% (fraca)', atual: true, melhor: true, redes: [{ nome: 'Vizinho2', sinal: 37, nivel: 'fraca', canal: 1 }], perto: [] },
        { rotulo: 'canal 11', canal: 11, disputa: 'média', pontos: 0.64, motivo: '3 redes', atual: false, melhor: false, redes: [{ nome: 'ESCRITORIO', sinal: 42, nivel: 'média', canal: 11 }, { nome: 'Rede_B3', sinal: 40, nivel: 'média', canal: 11 }, { nome: 'REDE_C', sinal: 40, nivel: 'média', canal: 11 }], perto: [{ nome: 'VIZINHO3', sinal: 35, nivel: 'fraca', canal: 10 }] },
        { rotulo: 'canal 6', canal: 6, disputa: 'muita', pontos: 1.07, motivo: '2 redes', atual: false, melhor: false, redes: [{ nome: 'Vizinho1', sinal: 90, nivel: 'forte', canal: 6 }, { nome: 'DDPAI', sinal: 40, nivel: 'média', canal: 6 }], perto: [] },
      ],
      canais: [
        { canal: 1, redes: null, mais_forte: 0, ocupacao: 0, proprio: true, candidato: true },
        { canal: 6, redes: ['Vizinho1 (90%)'], mais_forte: 90, ocupacao: 0.81, proprio: false, candidato: true },
      ],
    },
    faixa_5: {
      nome: '5 GHz', tua_rede: 'MINHA_REDE_5G', canal_atual: 48, recomendado: 36, mudar: true, vizinhos: 1,
      texto: 'Vale trocar para o canal 36: no 48 o ar é dividido com X (80%).',
      comparacao: [
        { rotulo: 'canais 36–48', canal: 48, disputa: 'muita', pontos: 0.64, motivo: '1 rede', atual: true, melhor: false, redes: [{ nome: 'X', sinal: 80, nivel: 'forte', canal: 48 }], perto: [] },
      ],
      canais: [
        { canal: 36, redes: null, mais_forte: 0, ocupacao: 0, proprio: false, candidato: true },
        { canal: 48, redes: ['X (80%)'], mais_forte: 80, ocupacao: 0.64, proprio: true, candidato: true },
        { canal: 52, redes: null, mais_forte: 0, ocupacao: 0, proprio: false, candidato: false, dfs: true },
      ],
    },
    buscas: 4,
    vistas: [
      { ssid: 'MINHA_REDE_5G', canal: 48, freq_mhz: 5240, sinal: 79, conectada: true, propria: true, seguranca: 'WPA2', sinal_medio: 79, vezes: 4, buscas: 4 },
      { ssid: 'Vizinho1', canal: 6, freq_mhz: 2437, sinal: 90, conectada: false, propria: false, seguranca: 'WPA2', sinal_medio: 91, vezes: 4, buscas: 4 },
      { ssid: 'longe', canal: 11, freq_mhz: 2462, sinal: 22, conectada: false, propria: false, seguranca: 'WPA2', sinal_medio: 25, vezes: 1, buscas: 4 },
    ],
  },
}

const relatorio: RelatorioSaude = {
  gerado_em: agoraISO,
  nota: 82,
  rotulo: 'Boa',
  sem_dados: 1,
  itens: [
    { id: 'latencia', area: 'Internet', titulo: 'Tempo de resposta', estado: 'ok', resumo: 'Resposta rápida: 48 ms.', prova: 'mediana de 48 ms' },
    { id: 'fila', area: 'Internet', titulo: 'Internet ocupada (fila cheia)', estado: 'problema', resumo: 'A internet trava.', prova: 'nota C', recomendacao: 'Ligue o SQM no roteador.', link: '/internet' },
    { id: 'pico', area: 'Internet', titulo: 'Horário de pico', estado: 'sem_dados', resumo: 'Precisa de 1 a 2 dias.' },
    { id: 'memoria', area: 'Computador', titulo: 'Memória', estado: 'ok', resumo: 'Sobra memória: 5,4 GB livres.' },
    { id: 'seguranca', area: 'Wi-Fi da casa', titulo: 'Proteção do Wi-Fi', estado: 'dica', resumo: 'WPA2.', recomendacao: 'Use WPA3 se tiver.' },
  ],
  recomendacoes: [
    { id: 'fila', area: 'Internet', titulo: 'Internet ocupada (fila cheia)', estado: 'problema', resumo: 'A internet trava.', recomendacao: 'Ligue o SQM no roteador.', link: '/internet' },
    { id: 'seguranca', area: 'Wi-Fi da casa', titulo: 'Proteção do Wi-Fi', estado: 'dica', resumo: 'WPA2.', recomendacao: 'Use WPA3 se tiver.' },
  ],
}

const computadorResp = {
  disponivel: true,
  resumo: {
    agora: { mem_total_mb: 7808, mem_disponivel_mb: 5524, swap_total_mb: 0, swap_usado_mb: 0, pressao_mem: 0, pressao_cpu: 0.4, pressao_disco: 0.4,
      carga_5: 0.4, nucleos: 4, discos: [{ ponto: '/', total_gb: 443, livre_gb: 280, uso_pct: 37 }], temp_cpu: 56, ligado_s: 17280 },
    temp_max_hora: 61,
    mem_min_hora_mb: 5000,
  },
}

const padrao = {
  '/saude-da-rede': relatorio,
  '/computador': computadorResp,
  '/wifi/vizinhanca/buscar': {},
  '/wifi/vizinhanca': vizinhancaResp,
  '/dispositivos/movimentos': {
    janela_min: 10,
    movimentos: [
      { tipo: 'entrou', em: '2026-10-29T14:30:36Z', dispositivo_id: 3, ip: '10.0.0.77', nome: 'Aparelho Samsung' },
      { tipo: 'saiu', em: '2026-10-29T12:05:00Z', dispositivo_id: 2, ip: '10.0.0.28', nome: 'Aparelho sem nome (MAC aleatório)' },
      { tipo: 'desligado', em: '2026-10-29T11:47:00Z', ate: '2026-10-29T12:00:00Z', dispositivo_id: 0, ip: '', nome: '' },
    ],
  },
  '/dispositivos/varrer': {},
  '/dispositivos/': {
    dispositivo: listaDisp.dispositivos[2],
    aparicoes: [{ inicio: '2026-10-29T17:00:00Z', fim: '2026-10-29T18:00:00Z', ip: '10.0.0.77' }],
    resumo_7d: { entradas: 14, segundos: 31 * 3600, media_s: 31 * 3600 / 14, hora_chegada: 18 },
  },
  '/dispositivos': listaDisp,
  '/aparelhos-novos': { dispositivos: [] },
  '/velocidade/testar': {},
  '/velocidade': velocidade,
  '/wifi': wifiResp,
  '/configuracao': { ajustes: ajustesAtuais, padrao: ajustesAtuais, estimativa_mes_mb: 3150 },
  '/visao-geral': comMedicao,
  '/internet/historico': historico,
  '/internet/disponibilidade': disponibilidade,
  '/quedas': { quedas: [] },
}

function roteador() {
  return createRouter({
    history: createMemoryHistory(),
    routes: [
      { path: '/', component: { template: '<div />' } },
      { path: '/entrar', name: 'entrar', component: { template: '<div />' } },
      { path: '/:x(.*)*', component: { template: '<div />' } },
    ],
  })
}

async function montar(c: typeof VisaoGeral) {
  const r = roteador()
  await r.push('/')
  const w = mount(c, { global: { plugins: [r] } })
  await flushPromises()
  return { w, r }
}

afterEach(() => {
  vi.unstubAllGlobals()
  localStorage.clear()
})

describe('Visão geral', () => {
  it('mostra as medições reais', async () => {
    api(padrao)
    const { w } = await montar(VisaoGeral)
    const texto = w.text()
    expect(texto).toContain('6,8 ms') // roteador
    expect(texto).toContain('30 ms') // internet
    expect(texto).toContain('Tudo normal')
    expect(texto).toContain('No ar')
    expect(texto).toContain('31 ms') // DNS configurado
    expect(texto).toContain('99,88%')
    expect(texto).toContain('Medido em 50% do período')
    expect(texto).toContain('sinal 79% (ótimo)')
    expect(texto).toContain('242 Mbps')
    expect(texto).toContain('85 Mbps')
    w.unmount()
  })

  it('o gráfico recebe os pontos e as três séries', async () => {
    api(padrao)
    const { w } = await montar(VisaoGeral)
    const g = w.find('.grafico-dublê')
    expect(g.attributes('data-pontos')).toBe('3')
    expect(g.attributes('data-series')).toBe('Roteador,Internet,DNS')
    w.unmount()
  })

  it('o que ainda não existe aparece como indisponível, sem números', async () => {
    api(padrao)
    const { w } = await montar(VisaoGeral)
    const blocos = w.findAllComponents(Indisponivel)
    expect(blocos.length).toBe(3) // dispositivos, anúncios, alertas
    w.unmount()
  })

  it('antes da primeira medição não inventa valor', async () => {
    const m = 'Aguardando a primeira medição.'
    api({
      ...padrao,
      '/visao-geral': {
        ...comMedicao,
        medido_em: null,
        interface: { disponivel: false, motivo: m, ativa: false, ok: false },
        gateway: indisponivel(m),
        internet: indisponivel(m),
        dns: indisponivel(m),
        wifi: indisponivel('Aguardando a primeira leitura.'),
        velocidade: { disponivel: false, motivo: 'Nenhum teste de velocidade ainda.', rodando: false, proximo: null },
      },
      '/internet/historico': { ...historico, gateway_ms: [null, null, null], internet_ms: [null, null, null], dns_ms: [null, null, null] },
      '/internet/disponibilidade': { '24h': indisponivel('Sem medições neste período.'), '7d': indisponivel('x'), '30d': indisponivel('x') },
    })
    const { w } = await montar(VisaoGeral)
    expect(w.text()).toContain(m)
    expect(w.text()).toContain('Ainda sem medições na última hora.')
    expect(w.text()).toContain('Sem medições neste período.')
    expect(w.text()).not.toMatch(/\d+(,\d)? ms/)
    expect(w.text()).not.toMatch(/\d%/)
    expect(w.find('.grafico-dublê').exists()).toBe(false)
    w.unmount()
  })

  it('queda em andamento aparece no topo com a causa', async () => {
    api({
      ...padrao,
      '/visao-geral': {
        ...comMedicao,
        internet: { ...comMedicao.internet, estado: 'fora', causa: { causa: 'externo', texto: 'Sem internet: o roteador responde, mas nada de fora' }, latencia_media_ms: null, perda: 1 },
        queda: {
          id: 7,
          inicio: agoraISO,
          fim: null,
          duracao_s: 95,
          em_andamento: true,
          causa: { causa: 'externo', texto: 'Sem internet: o roteador responde, mas nada de fora' },
          causas: [],
        },
      },
    })
    const { w } = await montar(VisaoGeral)
    const alerta = w.find('.queda')
    expect(alerta.text()).toContain('Sem internet')
    expect(alerta.text()).toContain('1 min 35 s')
    expect(w.text()).toContain('Fora')
    w.unmount()
  })

  it('erro do servidor mostra aviso com tentar de novo', async () => {
    let falhar = true
    api({
      ...padrao,
      '/visao-geral': () => (falhar ? respostaJSON(500, { erro: 'erro interno' }) : respostaJSON(200, comMedicao)),
    })
    const { w } = await montar(VisaoGeral)
    expect(w.find('[role="alert"]').text()).toContain('erro interno')
    falhar = false
    await w.find('[role="alert"] button').trigger('click')
    await flushPromises()
    expect(w.find('[role="alert"]').exists()).toBe(false)
    expect(w.text()).toContain('6,8 ms')
    w.unmount()
  })

  it('sessão expirada leva ao login', async () => {
    api({ '/': () => respostaJSON(401, { erro: 'sessão expirada' }) })
    const { w, r } = await montar(VisaoGeral)
    expect(r.currentRoute.value.name).toBe('entrar')
    w.unmount()
  })
})

describe('Internet', () => {
  it('mostra disponibilidade, gráficos e cada verificação', async () => {
    api(padrao)
    const { w } = await montar(Internet)
    const texto = w.text()
    expect(texto).toContain('Últimos 30 dias')
    expect(texto).toContain('Sem medições neste período.') // 30 dias sem dado não vira 0% nem 100%
    expect(w.findAll('.grafico-dublê').map((g) => g.attributes('data-series'))).toEqual([
      'Download,Upload',
      'Sinal médio,Pior momento',
      'Roteador,Internet,DNS',
      'Roteador,Internet',
    ])
    expect(texto).toContain('Nenhuma queda registrada')
    expect(texto).toContain('8.8.8.8')
    expect(texto).toContain('sem resposta')
    expect(texto).toContain('www.google.com/generate_204')
    w.unmount()
  })

  it('trocar o período busca de novo', async () => {
    const f = api(padrao)
    const { w } = await montar(Internet)
    const botao = w.findAll('button.periodo').find((b) => b.text() === '7 dias')!
    await botao.trigger('click')
    await flushPromises()
    expect(botao.attributes('aria-pressed')).toBe('true')
    const urls = f.mock.calls.map((c) => c[0])
    expect(urls).toContain('/api/v1/internet/historico?periodo=7d')
    expect(urls).toContain('/api/v1/quedas?periodo=7d')
    w.unmount()
  })

  it('lista as quedas com duração e causa', async () => {
    api({
      ...padrao,
      '/quedas': {
        quedas: [
          {
            id: 1,
            inicio: '2026-10-29T11:18:44Z',
            fim: '2026-10-29T11:19:24Z',
            duracao_s: 40,
            em_andamento: false,
            causa: { causa: 'rede_local', texto: 'Rede local: este aparelho está sem rede (Wi-Fi ou cabo)' },
            causas: [{ causa: 'rede_local', texto: 'Rede local: este aparelho está sem rede (Wi-Fi ou cabo)' }],
          },
        ],
      },
    })
    const { w } = await montar(Internet)
    expect(w.text()).toContain('Resolvida')
    expect(w.text()).toContain('40 s')
    expect(w.text()).toContain('Rede local')
    w.unmount()
  })
})

describe('Velocidade e Wi-Fi', () => {
  it('mostra o último teste, o gasto e o Wi-Fi', async () => {
    api(padrao)
    const { w } = await montar(Internet)
    const texto = w.text()
    expect(texto).toContain('242 Mbps')
    expect(texto).toContain('3× por dia')
    expect(texto).toContain('3,1 GB/mês')
    expect(texto).toContain('MINHA_REDE_5G')
    expect(texto).toContain('5 GHz · canal 48')
    expect(texto).toContain('Sinal ótimo')
    w.unmount()
  })

  it('testar agora pede confirmação antes', async () => {
    const f = api(padrao)
    const { w } = await montar(Internet)
    const posts = () => f.mock.calls.filter((c) => c[1]?.method === 'POST').map((c) => c[0])
    await w.findAll('button').find((b) => b.text() === 'Testar agora')!.trigger('click')
    expect(posts()).toEqual([]) // só abriu a confirmação
    await w.findAll('dialog button').find((b) => b.text() === 'Testar')!.trigger('click')
    await flushPromises()
    expect(posts()).toEqual(['/api/v1/velocidade/testar'])
    w.unmount()
  })

  it('sem teste ainda não inventa número', async () => {
    api({ ...padrao, '/velocidade': { ...velocidade, estado: { rodando: false, proximo: null, ultimo: null, progresso: null }, testes: [] } })
    const { w } = await montar(Internet)
    const cartao = w.findAll('section').find((c) => c.text().startsWith('Velocidade da internet'))!
    expect(cartao.text()).toContain('Nenhum teste ainda')
    expect(cartao.text()).not.toMatch(/\d+ Mbps/)
    w.unmount()
  })
})

describe('Configurações', () => {
  it('salva só depois de confirmar e manda os valores convertidos', async () => {
    let enviado: Ajustes | null = null
    api({
      '/configuracao': (op?: RequestInit) => {
        if (op?.method === 'PUT') {
          enviado = JSON.parse(String(op.body))
          return respostaJSON(200, { ajustes: enviado, padrao: ajustesAtuais, estimativa_mes_mb: 0 })
        }
        return respostaJSON(200, { ajustes: ajustesAtuais, padrao: ajustesAtuais, estimativa_mes_mb: 3150 })
      },
    })
    const { w } = await montar(Configuracoes)
    const salvar = () => w.find('button[type="submit"]')
    expect(salvar().attributes('disabled')).toBeDefined() // nada mudou ainda

    await w.find('input[type="number"]').setValue(30)
    await w.find('textarea').setValue('1.1.1.1\n\n  8.8.8.8  \n')
    expect(salvar().attributes('disabled')).toBeUndefined()
    await w.find('form').trigger('submit')
    expect(enviado).toBeNull() // pediu confirmação
    expect(w.find('dialog').text()).toContain('Intervalo entre verificações')
    await w.findAll('dialog button').find((b) => b.text() === 'Salvar')!.trigger('click')
    await flushPromises()
    expect(enviado!.intervalo_s).toBe(30)
    expect(enviado!.externos).toEqual(['1.1.1.1', '8.8.8.8'])
    expect(enviado!.limite_perda).toBe(0.1)
    expect(w.text()).toContain('Já está valendo')
    w.unmount()
  })

  it('mostra cada erro que o servidor apontou', async () => {
    api({
      '/configuracao': (op?: RequestInit) =>
        op?.method === 'PUT'
          ? respostaJSON(400, { erro: 'Algumas configurações não são válidas.', erros: ['intervalo: de 5 a 300 segundos', 'endereço externo 10.0.0.1 é da rede local'] })
          : respostaJSON(200, { ajustes: ajustesAtuais, padrao: ajustesAtuais, estimativa_mes_mb: 3150 }),
    })
    const { w } = await montar(Configuracoes)
    await w.find('input[type="number"]').setValue(1)
    await w.find('form').trigger('submit')
    await w.findAll('dialog button').find((b) => b.text() === 'Salvar')!.trigger('click')
    await flushPromises()
    const erros = w.find('.erros').findAll('li').map((li) => li.text())
    expect(erros).toEqual(['intervalo: de 5 a 300 segundos', 'endereço externo 10.0.0.1 é da rede local'])
    w.unmount()
  })

  it('o gasto estimado acompanha o formulário', async () => {
    api(padrao)
    const { w } = await montar(Configuracoes)
    expect(w.text()).toContain('3,1 GB por mês')
    await w.find('input[type="checkbox"]').setValue(false)
    expect(w.text()).toContain('0 MB por mês')
    w.unmount()
  })
})

describe('Wi-Fi da vizinhança', () => {
  it('mostra a recomendação de cada faixa e destaca o teu canal', async () => {
    api(padrao)
    const { w } = await montar(Internet)
    const card = w.findAll('section').find((s) => s.text().startsWith('Wi-Fi da vizinhança'))!
    expect(card.text()).toContain('3 redes por perto')
    expect(card.text()).toContain('média das últimas 4 buscas')
    expect(card.text()).toContain('Vale trocar para o 36')
    expect(card.text()).toContain('Canal bom')
    expect(card.text()).toContain('Canal 1 está livre de vizinhos. Não mexa.')
    const proprios = card.findAll('.coluna.proprio').map((c) => c.find('.num').text())
    expect(proprios).toEqual(['48', '1'])
    expect(card.find('.coluna[title^="Canal 6"]').attributes('title')).toContain('Vizinho1 (90%)')
    expect(card.findAll('.coluna.dfs').length).toBe(1)
    w.unmount()
  })

  it('compara os canais do melhor para o pior, com o porquê', async () => {
    api(padrao)
    const { w } = await montar(Internet)
    const itens = w.findAll('.faixa')[1].findAll('.comparacao > li').map((li) => li.text())
    expect(itens[0]).toContain('canal 1')
    expect(itens[0]).toContain('pouca disputa')
    expect(itens[0]).toContain('teu atual')
    expect(itens[0]).toContain('melhor')
    const linhasDo11 = w.findAll('.faixa')[1].findAll('.comparacao > li')[1].findAll('.vizinhos li').map((l) => l.text())
    expect(linhasDo11).toEqual(['ESCRITORIO42% · média', 'Rede_B340% · média', 'REDE_C40% · média'])
    expect(itens[1]).toContain('+ 1 em canal vizinho que se sobrepõe (VIZINHO3 35%)')
    expect(itens[2]).toContain('muita disputa')
    expect(w.findAll('.faixa')[1].findAll('.comparacao > li')[2].find('.medidor span').classes()).toContain('forte')
    w.unmount()
  })

  it('lista os vizinhos sempre aberta, com o sinal e em quantas buscas apareceram', async () => {
    api(padrao)
    const { w } = await montar(Internet)
    const linhas = w.findAll('.redes li')
    expect(linhas.length).toBe(3)
    expect(linhas[0].classes()).toContain('tua')
    expect(linhas[1].text()).toContain('Vizinho1')
    expect(linhas[1].text()).toContain('91% forte')
    expect(linhas[1].text()).toContain('2,4 GHz · canal 6')
    expect(linhas[2].text()).toContain('25% fraca')
    expect(linhas[2].text()).toContain('1/4')
    expect(linhas[1].find('.medidor span').classes()).toContain('forte')
    expect(w.text()).toContain('3 redes por perto')
    w.unmount()
  })

  it('sem a busca autorizada mostra o motivo', async () => {
    api({ ...padrao, '/wifi/vizinhanca': { disponivel: false, motivo: 'Não deu para buscar as redes: o sistema não autorizou', buscando: false } })
    const { w } = await montar(Internet)
    expect(w.text()).toContain('o sistema não autorizou')
    w.unmount()
  })

  it('"Buscar agora" pede a busca', async () => {
    const f = api(padrao)
    const { w } = await montar(Internet)
    await w.findAll('button').find((b) => b.text() === 'Buscar agora')!.trigger('click')
    await flushPromises()
    expect(f.mock.calls.some((c) => c[0] === '/api/v1/wifi/vizinhanca/buscar' && c[1]?.method === 'POST')).toBe(true)
    w.unmount()
  })
})

describe('Dispositivos', () => {
  it('lista, resume e avisa do aparelho novo', async () => {
    api(padrao)
    const { w } = await montar(Dispositivos)
    const texto = w.text()
    expect(texto).toContain('2 na rede agora')
    expect(texto).toContain('3 já vistos')
    expect(texto).toContain('Um aparelho novo apareceu na rede')
    expect(texto).toContain('MAC aleatório (privacidade)')
    expect(texto).toContain('procura automática está desligada')
    expect(texto).toContain('observados')
    w.unmount()
  })

  it('filtra por "Na rede", "Novos" e pela busca', async () => {
    api(padrao)
    const { w } = await montar(Dispositivos)
    const linhas = () => w.findAll('.lista .nome').map((n) => n.text())
    expect(linhas().length).toBe(3)
    await w.findAll('button.filtro').find((b) => b.text() === 'Na rede')!.trigger('click')
    expect(linhas().length).toBe(2)
    await w.findAll('button.filtro').find((b) => b.text() === 'Novos')!.trigger('click')
    expect(linhas()).toEqual([expect.stringContaining('Aparelho Samsung')])
    await w.findAll('button.filtro').find((b) => b.text() === 'Todos')!.trigger('click')
    await w.find('input.busca').setValue('intelbras')
    expect(linhas()).toEqual([expect.stringContaining('meuintelbras')])
    w.unmount()
  })

  it('"É conhecido" marca o aparelho novo como confiável', async () => {
    const f = api(padrao)
    const { w } = await montar(Dispositivos)
    await w.findAll('.novos button').find((b) => b.text() === 'É conhecido')!.trigger('click')
    await flushPromises()
    const patch = f.mock.calls.find((c) => c[1]?.method === 'PATCH')!
    expect(patch[0]).toBe('/api/v1/dispositivos/3')
    expect(JSON.parse(String(patch[1]!.body))).toEqual({ confiavel: true })
    w.unmount()
  })

  it('ligar a procura pede confirmação e salva só a opção', async () => {
    const enviados: { url: string; metodo: string; corpo: unknown }[] = []
    api({
      ...padrao,
      '/configuracao': (op?: RequestInit) => {
        if (op?.method === 'PUT') enviados.push({ url: 'configuracao', metodo: 'PUT', corpo: JSON.parse(String(op.body)) })
        return respostaJSON(200, { ajustes: ajustesAtuais, padrao: ajustesAtuais, estimativa_mes_mb: 0 })
      },
    })
    const { w } = await montar(Dispositivos)
    await w.findAll('button').find((b) => b.text() === 'Ligar procura')!.trigger('click')
    expect(enviados).toEqual([])
    await w.findAll('dialog button').find((b) => b.text() === 'Ligar')!.trigger('click')
    await flushPromises()
    expect(enviados[0].corpo).toEqual({ ...ajustesAtuais, varredura_ativa: true })
    expect(w.text()).toContain('Procura automática ligada')
    w.unmount()
  })

  it('lista e ficha mostram quantas vezes entrou e quanto tempo ficou', async () => {
    api(padrao)
    const { w } = await montar(Dispositivos)
    const samsung = w.findAll('.lista .linha')[2]
    expect(samsung.text()).toContain('14× em 7 dias · 31 h')
    await samsung.trigger('click')
    await flushPromises()
    const r = w.find('.resumo7d').text()
    expect(r).toContain('14 vezes')
    expect(r).toContain('1 dia 7 h')
    expect(r).toContain('2 h 12 min')
    expect(r).toContain('por volta das 18h')
    w.unmount()
  })

  it('detalhe mostra quando esteve na rede e salva o apelido', async () => {
    const f = api(padrao)
    const { w } = await montar(Dispositivos)
    await w.findAll('.lista .linha')[2].trigger('click')
    await flushPromises()
    expect(w.find('.detalhe').text()).toContain('00:11:22:33:44:55')
    expect(w.find('.aparicoes').text()).toContain('(1 h)')
    await w.find('.detalhe input').setValue('TV da sala')
    await w.findAll('.detalhe button').find((b) => b.text() === 'Salvar')!.trigger('click')
    await flushPromises()
    const patch = f.mock.calls.find((c) => c[1]?.method === 'PATCH')!
    expect(JSON.parse(String(patch[1]!.body))).toEqual({ apelido: 'TV da sala', notas: '' })
    w.unmount()
  })
})

describe('Saúde', () => {
  it('mostra a nota, o que fazer em ordem e cada área', async () => {
    api(padrao)
    const { w } = await montar(Saude)
    expect(w.find('.numero-nota').text()).toBe('82/100')
    expect(w.text()).toContain('Boa')
    expect(w.text()).toContain('4 de 5 pontos avaliados com dados reais')
    const fazer = w.findAll('.fazer li').map((li) => li.text())
    expect(fazer[0]).toContain('Problema')
    expect(fazer[0]).toContain('Ligue o SQM no roteador.')
    expect(fazer[1]).toContain('Dica')
    const titulos = w.findAll('.areas section').map((s) => s.find('h2').text())
    expect(titulos).toEqual(['Internet', 'Wi-Fi da casa', 'Computador'])
    expect(w.text()).toContain('mediana de 48 ms')
    w.unmount()
  })

  it('o estado aparece em texto, não só na cor', async () => {
    api(padrao)
    const { w } = await montar(Saude)
    const marcas = w.findAll('.itens .marca').map((m) => m.text())
    expect(marcas).toContain('✖Problema')
    expect(marcas).toContain('…Sem dados')
    w.unmount()
  })

  it('mostra os números do computador', async () => {
    api(padrao)
    const { w } = await montar(Saude)
    const t = w.find('.medidores').text()
    expect(t).toContain('5,4 de 7,6 GB')
    expect(t).toContain('37% usado · 280 GB livres')
    expect(t).toContain('56 °C (máx. 61 °C na hora)')
    expect(t).toContain('10% de 4 núcleos')
    w.unmount()
  })

  it('sem dados suficientes não mostra nota', async () => {
    api({ ...padrao, '/saude-da-rede': { ...relatorio, nota: null, rotulo: 'Ainda juntando dados', recomendacoes: null } })
    const { w } = await montar(Saude)
    expect(w.find('.numero-nota').text()).toBe('—')
    expect(w.text()).toContain('Ainda juntando dados')
    w.unmount()
  })

  it('a Visão geral leva para a Saúde', async () => {
    api(padrao)
    const { w } = await montar(VisaoGeral)
    expect(w.find('.saude-faixa').text()).toContain('Saúde: Boa')
    expect(w.find('.saude-faixa').text()).toContain('internet ocupada (fila cheia)')
    w.unmount()
  })
})

describe('Procura ao vivo e entradas/saídas', () => {
  it('quem não respondeu à última procura fica amarelo na hora, sem esperar 10 min', async () => {
    const inicio = new Date(Date.now() - 60_000).toISOString()
    api({
      ...padrao,
      '/dispositivos': {
        ...listaDisp,
        estado: { ...listaDisp.estado, ultima_varredura: new Date().toISOString(), ultima_varredura_inicio: inicio },
        dispositivos: [
          disp({ id: 1, nome: 'meuintelbras', ip: '10.0.0.1', ultimo_visto: new Date().toISOString() }),
          disp({ id: 5, nome: 'Celular-Android', ip: '10.0.0.2', ultimo_visto: new Date(Date.now() - 150_000).toISOString() }),
        ],
      },
    })
    const { w } = await montar(Dispositivos)
    const linhas = w.findAll('.lista .linha')
    expect(linhas[0].find('.ponto').classes()).toContain('agora')
    expect(linhas[1].find('.ponto').classes()).toContain('sumiu')
    expect(linhas[1].text()).toContain('não respondeu')
    expect(w.find('.resumo').text()).toContain('1 na rede agora')
    w.unmount()
  })

  it('mostra a barra da procura, quem respondeu e quem acabou de aparecer', async () => {
    let chamada = 0
    const varrendo = (feitos: number, achados: number, naRede2: boolean): ListaDispositivos => ({
      ...listaDisp,
      estado: { ...listaDisp.estado, varrendo: true, progresso: { feitos, total: 253, achados, inicio: agoraISO } },
      dispositivos: listaDisp.dispositivos.map((d) => (d.id === 2 ? { ...d, na_rede: naRede2 } : d)),
    })
    api({
      ...padrao,
      '/dispositivos/movimentos': { janela_min: 10, movimentos: [] },
      '/dispositivos': () => {
        chamada++
        return respostaJSON(200, chamada === 1 ? varrendo(64, 3, false) : varrendo(128, 4, true))
      },
    })
    vi.useFakeTimers()
    const { w } = await montar(Dispositivos)
    expect(w.find('.procura').text()).toContain('64 de 253 endereços · 3 aparelhos responderam')
    expect(w.find('[role="progressbar"]').attributes('aria-valuenow')).toBe('25')
    await vi.advanceTimersByTimeAsync(1600)
    await flushPromises()
    expect(w.find('.procura').text()).toContain('128 de 253')
    expect(w.find('.procura').text()).toContain('1 apareceu agora')
    const chegou = w.find('.linha.chegou')
    expect(chegou.text()).toContain('acabou de aparecer')
    expect(chegou.text()).toContain('Aparelho sem nome')
    w.unmount()
    vi.useRealTimers()
  })

  it('lista quem entrou e quem saiu, e abre a ficha pelo nome', async () => {
    api(padrao)
    const { w } = await montar(Dispositivos)
    const linhas = w.findAll('.movimentos li').map((l) => l.text())
    expect(linhas[0]).toContain('● entrou')
    expect(linhas[0]).toContain('Aparelho Samsung')
    expect(linhas[1]).toContain('○ saiu')
    expect(linhas[2]).toContain('◌ parado')
    expect(linhas[2]).toContain('NetGuard desligado até')
    expect(w.findAll('.mov-nome')).toHaveLength(2)
    await w.findAll('.mov-nome')[0].trigger('click')
    await flushPromises()
    expect(w.find('.detalhe h2').text()).toBe('Aparelho Samsung')
    w.unmount()
  })

  it('teste de velocidade mostra a fase e a velocidade subindo', async () => {
    api({
      ...padrao,
      '/velocidade': { ...velocidade, estado: { ...velocidade.estado, rodando: true, progresso: { fase: 'download', bytes: 15 * 1048576, total: 25 * 1048576, mbps_agora: 231.4 } } },
    })
    const { w } = await montar(Internet)
    const vivo = w.find('.ao-vivo').text()
    expect(vivo).toContain('✔ Resposta')
    expect(vivo).toContain('231 Mbps')
    expect(vivo).toContain('baixando · 15 de 25 MB')
    expect(w.find('.fase.atual').text()).toContain('Download')
    w.unmount()
  })
})

describe('Layout', () => {
  it('tem os 8 itens do menu, na ordem pedida', async () => {
    const w = mount(LayoutPainel, { global: { plugins: [roteador()] } })
    const itens = w.findAll('nav li').map((li) => li.text().replace(/^\S+\s*/, ''))
    expect(itens).toEqual([
      'Visão geral',
      'Saúde',
      'Internet',
      'Dispositivos',
      'Consumo de rede',
      'DNS e anúncios',
      'Alertas',
      'Histórico de eventos',
      'Configurações',
    ])
  })

  it('no celular o botão abre e fecha o menu', async () => {
    const w = mount(LayoutPainel, { global: { plugins: [roteador()] } })
    const botao = w.find('button.alternar')
    expect(botao.attributes('aria-expanded')).toBe('false')
    await botao.trigger('click')
    expect(w.find('.painel').classes()).toContain('menu-aberto')
    expect(botao.attributes('aria-expanded')).toBe('true')
    await w.find('.cortina').trigger('click')
    expect(w.find('.painel').classes()).not.toContain('menu-aberto')
  })
})
