<script setup lang="ts">
// Gráfico de linhas no tempo (uPlot). Um eixo só; buraco onde não houve medição;
// faixa vermelha nos intervalos em que a internet esteve fora (se `fora` vier).
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import uPlot from 'uplot'
import 'uplot/dist/uPlot.min.css'

export interface Serie {
  nome: string
  valores: (number | null)[]
  cor: 1 | 2 | 3 // posição na paleta (ordem fixa: roteador, internet, DNS)
}

const props = withDefaults(
  defineProps<{
    t: number[] // segundos (Unix)
    series: Serie[]
    unidade: 'ms' | '%' | 'Mbps' | 'sinal'
    fora?: (number | null)[]
    passoS?: number
    altura?: number
    rotulo: string // descrição para leitor de tela
  }>(),
  { altura: 220, passoS: 60 },
)

const caixa = ref<HTMLDivElement | null>(null)
let grafico: uPlot | null = null
let observador: ResizeObserver | null = null
let tema: MediaQueryList | null = null

const cor = (nome: string) => getComputedStyle(document.documentElement).getPropertyValue(nome).trim()

function formatarValor(v: number | null | undefined): string {
  if (v == null) return '—'
  if (props.unidade === '%') return `${(v * 100).toLocaleString('pt-BR', { maximumFractionDigits: 1 })}%`
  if (props.unidade === 'sinal') return `${Math.round(v)}%`
  if (props.unidade === 'Mbps') return `${v.toLocaleString('pt-BR', { maximumFractionDigits: v < 10 ? 1 : 0 })} Mbps`
  return `${v.toLocaleString('pt-BR', { maximumFractionDigits: v < 10 ? 1 : 0 })} ms`
}

function formatarHora(s: number, longo: boolean): string {
  const d = new Date(s * 1000)
  return longo
    ? d.toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit' })
    : d.toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })
}

function teto(max: number | null): number {
  if (props.unidade === '%') return Math.max(1, max ?? 1)
  if (props.unidade === 'sinal') return 100
  return Math.max(10, (max ?? 10) * 1.15)
}

// Ponto com dado entre dois buracos: a linha não aparece, então vira bolinha.
function isolados(u: uPlot, sidx: number): number[] {
  const v = u.data[sidx] as (number | null)[]
  const r: number[] = []
  for (let i = 0; i < v.length; i++) {
    if (v[i] != null && v[i - 1] == null && v[i + 1] == null) r.push(i)
  }
  return r
}

// Sem o mouse em cima, a legenda mostra o último valor medido.
function ultimo(valores: (number | null)[]): number | null {
  for (let i = valores.length - 1; i >= 0; i--) if (valores[i] != null) return valores[i]
  return null
}

// Pinta os intervalos com internet fora antes das linhas.
function faixaFora(u: uPlot) {
  const fora = props.fora
  if (!fora) return
  const ctx = u.ctx
  const { top, height } = u.bbox
  ctx.save()
  ctx.fillStyle = cor('--faixa-fora')
  for (let i = 0; i < props.t.length; i++) {
    const f = fora[i]
    if (!f) continue
    const x0 = u.valToPos(props.t[i], 'x', true)
    const x1 = u.valToPos(props.t[i] + props.passoS, 'x', true)
    ctx.fillRect(x0, top, Math.max(2, x1 - x0), height)
  }
  ctx.restore()
}

function montar() {
  const el = caixa.value
  if (!el) return
  grafico?.destroy()
  const span = props.t.length ? props.t[props.t.length - 1] - props.t[0] : 0
  const longo = span > 26 * 3600
  const eixo = cor('--texto-suave')
  const grade = cor('--borda')

  const opcoes: uPlot.Options = {
    width: el.clientWidth || 600,
    height: props.altura,
    cursor: { y: false, points: { size: 8 } },
    legend: { live: true },
    scales: { x: { time: true }, y: { range: (_u, _min, max) => [0, teto(max)] } },
    axes: [
      {
        stroke: eixo,
        grid: { stroke: grade, width: 1 },
        ticks: { show: false },
        values: (_u, splits) => splits.map((s) => formatarHora(s, longo)),
      },
      {
        stroke: eixo,
        grid: { stroke: grade, width: 1 },
        ticks: { show: false },
        size: 56,
        values: (_u, splits) => splits.map((v) => formatarValor(v)),
      },
    ],
    series: [
      { label: 'Horário', value: (_u, v, _s, idx) => (idx == null ? 'mais recente' : v == null ? '' : formatarHora(v, true)) },
      ...props.series.map((s) => ({
        label: s.nome,
        stroke: cor(`--serie-${s.cor}`),
        width: 2,
        spanGaps: false,
        points: { show: true, size: 6, fill: cor(`--serie-${s.cor}`), filter: isolados },
        value: (_u: uPlot, v: number | null, _s: number, idx: number | null) => formatarValor(idx == null ? ultimo(s.valores) : v),
      })),
    ],
    hooks: { drawClear: [faixaFora] },
  }
  grafico = new uPlot(opcoes, [props.t, ...props.series.map((s) => s.valores)] as uPlot.AlignedData, el)
}

onMounted(() => {
  montar()
  observador = new ResizeObserver(() => {
    if (grafico && caixa.value) grafico.setSize({ width: caixa.value.clientWidth, height: props.altura })
  })
  if (caixa.value) observador.observe(caixa.value)
  tema = window.matchMedia('(prefers-color-scheme: dark)')
  tema.addEventListener('change', montar)
})

onBeforeUnmount(() => {
  observador?.disconnect()
  tema?.removeEventListener('change', montar)
  grafico?.destroy()
  grafico = null
})

watch(() => [props.t, props.series, props.fora], montar)
</script>

<template>
  <figure class="grafico" :aria-label="rotulo">
    <div ref="caixa" class="caixa"></div>
    <figcaption v-if="fora" class="legenda-fora">
      <span class="amostra" aria-hidden="true"></span> faixa vermelha = internet fora
    </figcaption>
  </figure>
</template>

<style scoped>
.grafico {
  margin: 0;
  min-width: 0;
}
.caixa {
  width: 100%;
  min-width: 0;
}
.legenda-fora {
  display: flex;
  align-items: center;
  gap: 0.4rem;
  font-size: 0.78rem;
  color: var(--texto-suave);
  margin-top: 0.25rem;
}
.amostra {
  width: 0.9rem;
  height: 0.6rem;
  border-radius: 2px;
  background: var(--faixa-fora);
  border: 1px solid var(--erro);
}
/* legenda do uPlot no tema do painel */
.caixa :deep(.u-legend) {
  font-size: 0.82rem;
  color: var(--texto);
  text-align: left;
  margin-top: 0.3rem;
}
.caixa :deep(.u-legend .u-value) {
  font-variant-numeric: tabular-nums;
  color: var(--texto-suave);
}
.caixa :deep(.u-legend th) {
  font-weight: 500;
}
</style>
