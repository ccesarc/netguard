<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { pedir } from '../api/cliente'
import type { Disponibilidades, Historico, Periodo, Queda, VisaoGeral } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { milissegundos, porcentagem } from '../formatar'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import CartaoDisponibilidade from '../components/CartaoDisponibilidade.vue'
import GraficoSerie from '../components/GraficoSerie.vue'
import ListaQuedas from '../components/ListaQuedas.vue'
import SecaoVelocidade from '../components/SecaoVelocidade.vue'
import SecaoWifi from '../components/SecaoWifi.vue'
import SecaoVizinhanca from '../components/SecaoVizinhanca.vue'
import Indisponivel from '../components/Indisponivel.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

const expirou = useSessaoExpirada()

const opcoes: { valor: Periodo; texto: string }[] = [
  { valor: '1h', texto: '1 hora' },
  { valor: '6h', texto: '6 horas' },
  { valor: '24h', texto: '24 horas' },
  { valor: '7d', texto: '7 dias' },
  { valor: '30d', texto: '30 dias' },
]
const periodo = ref<Periodo>('24h')
try {
  const salvo = localStorage.getItem('netguard.periodo') as Periodo | null
  if (salvo && opcoes.some((o) => o.valor === salvo)) periodo.value = salvo
} catch {
  // sem armazenamento local: fica o padrão
}

const hist = usePeriodico(() => pedir<Historico>('GET', `/internet/historico?periodo=${periodo.value}`), 60_000, expirou)
const quedas = usePeriodico(
  () => pedir<{ quedas: Queda[] }>('GET', `/quedas?periodo=${periodo.value === '1h' || periodo.value === '6h' ? '24h' : periodo.value}`),
  60_000,
  expirou,
)
const disp = usePeriodico(() => pedir<Disponibilidades>('GET', '/internet/disponibilidade'), 60_000, expirou)
const agora = usePeriodico(() => pedir<VisaoGeral>('GET', '/visao-geral'), 10_000, expirou)

watch(periodo, (p) => {
  try {
    localStorage.setItem('netguard.periodo', p)
  } catch {
    // ignora
  }
  hist.atualizar()
  quedas.atualizar()
})

const h = computed(() => hist.dados.value)
const temPontos = computed(() => !!h.value && h.value.gateway_ms.some((v) => v !== null))
const seriesLatencia = computed(() =>
  h.value
    ? [
        { nome: 'Roteador', valores: h.value.gateway_ms, cor: 1 as const },
        { nome: 'Internet', valores: h.value.internet_ms, cor: 2 as const },
        { nome: 'DNS', valores: h.value.dns_ms, cor: 3 as const },
      ]
    : [],
)
const seriesPerda = computed(() =>
  h.value
    ? [
        { nome: 'Roteador', valores: h.value.gateway_perda, cor: 1 as const },
        { nome: 'Internet', valores: h.value.internet_perda, cor: 2 as const },
      ]
    : [],
)
const textoQuedas = computed(() =>
  periodo.value === '1h' || periodo.value === '6h' ? 'Quedas nas últimas 24 horas' : `Quedas — ${opcoes.find((o) => o.valor === periodo.value)?.texto}`,
)

const n = computed(() => {
  const v = agora.dados.value?.internet
  return v?.disponivel ? v : null
})
const dns = computed(() => {
  const v = agora.dados.value?.dns
  return v?.disponivel ? v : null
})
</script>

<template>
  <div class="cabecalho">
    <h1>Internet</h1>
    <div class="periodos" role="group" aria-label="Período">
      <button
        v-for="o in opcoes"
        :key="o.valor"
        type="button"
        class="periodo"
        :class="{ ativo: periodo === o.valor }"
        :aria-pressed="periodo === o.valor"
        @click="periodo = o.valor"
      >
        {{ o.texto }}
      </button>
    </div>
  </div>

  <ErroCarga v-if="hist.erro.value" :mensagem="hist.erro.value" class="bloco" @tentar="hist.atualizar" />

  <CartaoIndicador titulo="Disponibilidade" class="bloco">
    <div v-if="disp.dados.value" class="tres">
      <CartaoDisponibilidade titulo="Últimas 24 h" :d="disp.dados.value['24h']" />
      <CartaoDisponibilidade titulo="Últimos 7 dias" :d="disp.dados.value['7d']" />
      <CartaoDisponibilidade titulo="Últimos 30 dias" :d="disp.dados.value['30d']" />
    </div>
    <Carregando v-else />
    <template #rodape>
      "No ar" conta os ciclos em que a internet funcionava (lenta também conta). Períodos com o monitor
      desligado não entram na conta: aparecem em "medido em".
    </template>
  </CartaoIndicador>

  <div class="duas bloco">
    <SecaoVelocidade />
    <SecaoWifi :periodo="periodo" />
  </div>

  <SecaoVizinhanca class="bloco" />

  <CartaoIndicador titulo="Latência" class="bloco">
    <GraficoSerie
      v-if="h && temPontos"
      :t="h.t"
      :series="seriesLatencia"
      :fora="h.fora"
      :passo-s="h.passo_s"
      unidade="ms"
      :altura="260"
      rotulo="Latência do roteador, da internet e do DNS no período"
    />
    <Indisponivel v-else-if="h" motivo="Sem medições neste período." />
    <Carregando v-else />
    <template #rodape>
      Roteador = até o aparelho da operadora em casa · Internet = média até 1.1.1.1, 8.8.8.8 e 9.9.9.9 ·
      DNS = tempo de resposta do DNS configurado. Cada ponto é a média de
      {{ h ? (h.passo_s >= 3600 ? `${h.passo_s / 3600} h` : `${h.passo_s / 60} min`) : '' }}.
    </template>
  </CartaoIndicador>

  <div class="duas bloco">
    <CartaoIndicador titulo="Perda de pacotes">
      <GraficoSerie
        v-if="h && temPontos"
        :t="h.t"
        :series="seriesPerda"
        :passo-s="h.passo_s"
        unidade="%"
        :altura="180"
        rotulo="Perda de pacotes do roteador e da internet no período"
      />
      <Indisponivel v-else-if="h" motivo="Sem medições neste período." />
      <Carregando v-else />
    </CartaoIndicador>

    <CartaoIndicador :titulo="textoQuedas">
      <ListaQuedas v-if="quedas.dados.value" :quedas="quedas.dados.value.quedas" />
      <Carregando v-else />
    </CartaoIndicador>
  </div>

  <CartaoIndicador titulo="Agora — cada verificação" class="bloco">
    <Carregando v-if="!agora.dados.value" />
    <Indisponivel v-else-if="!n" motivo="Este aparelho está sem rede: nada a medir." />
    <div v-else class="tabela-rolagem">
      <table class="tabela">
        <thead>
          <tr>
            <th>Verificação</th>
            <th>Destino</th>
            <th>Resultado</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="e in n.externos" :key="'p' + e.alvo">
            <td>Ping{{ e.via_tcp ? ' (TCP 443)' : '' }}</td>
            <td class="mono">{{ e.alvo }}</td>
            <td :class="{ falha: !e.ok }">
              {{ e.ok ? milissegundos(e.latencia_media_ms) : e.erro || 'sem resposta' }}
              <span v-if="e.ok && e.perda > 0" class="suave"> · {{ porcentagem(e.perda) }} perda</span>
            </td>
          </tr>
          <tr v-for="m in dns?.sistema" :key="'ds' + m.alvo">
            <td>DNS configurado</td>
            <td class="mono">{{ m.alvo }}</td>
            <td :class="{ falha: !m.ok }">{{ m.ok ? milissegundos(m.ms) : m.erro }}</td>
          </tr>
          <tr v-for="m in dns?.externo" :key="'de' + m.alvo">
            <td>DNS de comparação</td>
            <td class="mono">{{ m.alvo }}</td>
            <td :class="{ falha: !m.ok }">{{ m.ok ? milissegundos(m.ms) : m.erro }}</td>
          </tr>
          <tr v-for="s in n.servicos" :key="'h' + s.alvo">
            <td>Site (HTTPS)</td>
            <td class="mono quebra">{{ s.alvo.replace(/^https:\/\//, '') }}</td>
            <td :class="{ falha: !s.ok }">{{ s.ok ? milissegundos(s.ms) : s.erro }}</td>
          </tr>
        </tbody>
      </table>
    </div>
  </CartaoIndicador>
</template>

<style scoped>
.cabecalho {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem 1rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}
h1 {
  font-size: 1.5rem;
}
.periodos {
  display: flex;
  flex-wrap: wrap;
  gap: 2px;
  background: var(--superficie-2);
  border: 1px solid var(--borda);
  border-radius: 8px;
  padding: 2px;
}
.periodo {
  border: 0;
  background: none;
  color: var(--texto-suave);
  border-radius: 6px;
  padding: 0.3rem 0.7rem;
  font-size: 0.88rem;
}
.periodo.ativo {
  background: var(--superficie);
  color: var(--texto);
  box-shadow: var(--sombra);
  font-weight: 600;
}
.bloco {
  margin-bottom: 1rem;
}
.tres {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 12rem), 1fr));
  gap: 1rem;
}
.duas {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
  gap: 1rem;
}
.tabela-rolagem {
  overflow-x: auto;
}
.tabela {
  width: 100%;
  border-collapse: collapse;
  font-size: 0.9rem;
}
.tabela th {
  text-align: left;
  font-weight: 500;
  color: var(--texto-suave);
  border-bottom: 1px solid var(--borda);
  padding: 0.4rem 0.5rem 0.4rem 0;
}
.tabela td {
  padding: 0.4rem 0.5rem 0.4rem 0;
  border-bottom: 1px solid var(--borda);
  font-variant-numeric: tabular-nums;
}
.tabela tr:last-child td {
  border-bottom: 0;
}
.falha {
  color: var(--erro);
  font-weight: 500;
}
.quebra {
  word-break: break-all;
}
</style>
