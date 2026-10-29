// Formatos devolvidos pela API (ver backend/internal/api).

export interface Usuario {
  nome: string
}

export interface Indisponivel {
  disponivel: false
  motivo: string
}

export interface InterfaceRede {
  disponivel: boolean
  motivo?: string
  nome?: string
  tipo?: 'wifi' | 'cabo' | 'outro'
  ativa: boolean
  ipv4?: string
  gateway?: string
  ok: boolean
  problema?: string
}

export interface MedidaPing {
  alvo: string
  ok: boolean
  enviados: number
  recebidos: number
  perda: number
  latencia_min_ms: number
  latencia_media_ms: number
  latencia_max_ms: number
  via_tcp?: boolean
  erro?: string
  medido_em: string
}

export interface Medida {
  alvo: string
  ok: boolean
  ms: number
  status?: number
  erro?: string
}

export type Gateway = Indisponivel | ({ disponivel: true } & MedidaPing)

export type EstadoRede = 'ok' | 'degradado' | 'fora'

export interface Causa {
  causa: string
  texto: string
}

export type Internet =
  | Indisponivel
  | {
      disponivel: true
      estado: EstadoRede
      causa: Causa
      detalhe?: string
      latencia_media_ms: number | null
      perda: number | null
      externos?: MedidaPing[]
      servicos?: Medida[]
    }

export type DNS =
  | Indisponivel
  | { disponivel: true; ok: boolean; sistema?: Medida[]; externo?: Medida[] }

export interface Queda {
  id: number
  inicio: string
  fim: string | null
  duracao_s: number
  em_andamento: boolean
  causa: Causa
  causas: Causa[]
  detalhe?: string
}

export interface VisaoGeral {
  medido_em: string | null
  interface: InterfaceRede
  gateway: Gateway
  internet: Internet
  dns: DNS
  queda: Queda | null
  wifi: WifiAtual
  velocidade:
    | (Indisponivel & { rodando: boolean; proximo: string | null })
    | { disponivel: true; rodando: boolean; ultimo: ResultadoVelocidade; proximo: string | null }
  dispositivos: Indisponivel | { disponivel: true; resumo: ResumoDispositivos; varredura_ativa: boolean }
  bloqueio: Indisponivel
  alertas: Indisponivel
}

export type Periodo = '1h' | '6h' | '24h' | '7d' | '30d'

export interface Historico {
  periodo: Periodo
  passo_s: number
  t: number[]
  gateway_ms: (number | null)[]
  gateway_perda: (number | null)[]
  internet_ms: (number | null)[]
  internet_perda: (number | null)[]
  dns_ms: (number | null)[]
  fora: (number | null)[]
}

export type Disponibilidade =
  | Indisponivel
  | {
      disponivel: true
      percentual: number
      cobertura: number
      quedas: number
      tempo_fora_s: number
      ciclos: number
    }

export type Disponibilidades = Record<'24h' | '7d' | '30d', Disponibilidade>

export interface ResultadoVelocidade {
  em: string
  origem: 'agendado' | 'manual'
  download_mbps: number | null
  upload_mbps: number | null
  latencia_ms: number | null
  bytes_baixados: number
  bytes_enviados: number
  duracao_s: number
  servidor?: string
  interface?: string
  erro?: string
}

export interface ConfigVelocidade {
  ativo: boolean
  vezes_por_dia: number
  mb_download: number
  mb_upload: number
}

export interface ProgressoVelocidade {
  fase: 'resposta' | 'download' | 'upload'
  bytes: number
  total: number
  mbps_agora: number
}

export interface EstadoVelocidade {
  rodando: boolean
  proximo: string | null
  ultimo: ResultadoVelocidade | null
  progresso: ProgressoVelocidade | null
}

export interface Velocidade {
  estado: EstadoVelocidade
  testes: ResultadoVelocidade[]
  gasto_30d_mb: number
  estimativa_mes_mb: number
  config: ConfigVelocidade
}

export interface AmostraWifi {
  em: string
  ssid: string
  sinal: number
  canal: number
  freq_mhz: number
  taxa_mbps: number
  seguranca?: string
  redes_mesmo_canal: number
  redes_vistas: number
}

export type WifiAtual = Indisponivel | { disponivel: true; amostra: AmostraWifi }

export interface Wifi {
  atual: WifiAtual
  historico: { passo_s: number; t: number[]; sinal: (number | null)[]; sinal_min: (number | null)[]; taxa_mbps: (number | null)[] }
}

export interface Ajustes {
  intervalo_s: number
  externos: string[]
  dns_comparacao: string
  urls: string[]
  limite_latencia_ms: number
  limite_perda: number
  velocidade: ConfigVelocidade
  varredura_ativa: boolean
  plano_download_mbps: number
  plano_upload_mbps: number
}

export interface Configuracao {
  ajustes: Ajustes
  padrao: Ajustes
  estimativa_mes_mb: number
}

export interface Dispositivo {
  id: number
  mac: string
  ip: string
  nome: string
  nome_descoberto: string
  apelido: string
  notas: string
  fabricante: string
  mac_aleatorio: boolean
  confiavel: boolean
  este_aparelho: boolean
  alertar_novo: boolean
  na_rede: boolean
  primeiro_visto: string
  ultimo_visto: string
  presenca_7d?: { entradas: number; segundos: number }
}

export interface ResumoPresenca {
  entradas: number
  segundos: number
  media_s: number
  hora_chegada: number | null
}

export interface ResumoDispositivos {
  total: number
  na_rede: number
  novos: number
}

export interface EstadoDescoberta {
  ultima_coleta: string | null
  ultima_varredura: string | null
  ultima_varredura_inicio: string | null
  varrendo: boolean
  varredura_ativa: boolean
  progresso: { feitos: number; total: number; achados: number; inicio: string } | null
}

export interface Movimento {
  tipo: 'entrou' | 'saiu' | 'desligado'
  em: string
  ate?: string // só no "desligado": quando o NetGuard voltou
  dispositivo_id: number
  ip: string
  nome: string
}

export interface ListaDispositivos {
  estado: EstadoDescoberta
  resumo: ResumoDispositivos
  dispositivos: Dispositivo[]
}

export interface Aparicao {
  inicio: string
  fim: string
  ip: string
}

export interface RedeVizinha {
  ssid: string
  canal: number
  freq_mhz: number
  sinal: number
  seguranca?: string
  conectada: boolean
  propria: boolean
}

export interface RedeVista extends RedeVizinha {
  sinal_medio: number
  vezes: number
  buscas: number
}

export interface OcupacaoCanal {
  canal: number
  redes: string[] | null
  mais_forte: number
  ocupacao: number
  proprio: boolean
  candidato: boolean
  dfs?: boolean
}

export interface VizinhoCanal {
  nome: string
  sinal: number
  nivel: 'forte' | 'média' | 'fraca'
  canal: number
}

export interface CandidatoCanal {
  rotulo: string
  canal: number
  disputa: 'pouca' | 'média' | 'muita'
  pontos: number
  motivo: string
  atual: boolean
  melhor: boolean
  dfs?: boolean
  redes: VizinhoCanal[]
  perto: VizinhoCanal[]
}

export interface FaixaWifi {
  nome: string
  tua_rede?: string
  canal_atual: number
  recomendado: number
  mudar: boolean
  texto: string
  canais: OcupacaoCanal[]
  vizinhos: number
  comparacao: CandidatoCanal[]
}

export type Vizinhanca =
  | (Indisponivel & { buscando: boolean; buscando_desde?: string | null })
  | { disponivel: true; buscando: boolean; buscando_desde?: string | null; vizinhanca: { em: string; redes: RedeVizinha[]; vistas?: RedeVista[]; faixa_24: FaixaWifi; faixa_5: FaixaWifi; buscas: number } }

export type EstadoSaude = 'ok' | 'dica' | 'atencao' | 'problema' | 'sem_dados'

export interface ItemSaude {
  id: string
  area: string
  titulo: string
  estado: EstadoSaude
  resumo: string
  prova?: string
  recomendacao?: string
  link?: string
}

export interface RelatorioSaude {
  gerado_em: string
  nota: number | null
  rotulo: string
  itens: ItemSaude[]
  recomendacoes: ItemSaude[] | null
  sem_dados: number
}

export interface LeituraComputador {
  mem_total_mb: number
  mem_disponivel_mb: number
  swap_total_mb: number
  swap_usado_mb: number
  pressao_mem: number | null
  pressao_cpu: number | null
  pressao_disco: number | null
  carga_5: number
  nucleos: number
  discos: { ponto: string; total_gb: number; livre_gb: number; uso_pct: number }[] | null
  temp_cpu: number | null
  temp_sensor?: string
  ligado_s: number
}

export type Computador =
  | Indisponivel
  | { disponivel: true; resumo: { agora: LeituraComputador; temp_max_hora: number | null; mem_min_hora_mb: number } }
