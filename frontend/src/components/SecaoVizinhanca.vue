<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ErroApi, pedir } from '../api/cliente'
import type { FaixaWifi, Vizinhanca } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { haQuantoTempo } from '../formatar'
import CartaoIndicador from './CartaoIndicador.vue'
import Indisponivel from './Indisponivel.vue'
import Carregando from './Carregando.vue'
import SeloEstado from './SeloEstado.vue'

const { dados, atualizar } = usePeriodico(() => pedir<Vizinhanca>('GET', '/wifi/vizinhanca'), 60_000, useSessaoExpirada())
const aviso = ref<string | null>(null)
const verTodas = ref(false)
const limite = 12

const viz = computed(() => (dados.value?.disponivel ? dados.value.vizinhanca : null))
const faixas = computed<FaixaWifi[]>(() => (viz.value ? [viz.value.faixa_5, viz.value.faixa_24] : []))
// Vistas nas últimas buscas (servidor novo); senão, só as da última busca.
const vistas = computed(() =>
  viz.value?.vistas ?? (viz.value?.redes ?? []).map((r) => ({ ...r, sinal_medio: r.sinal, vezes: 1, buscas: 1 })),
)
const vistasMostradas = computed(() => (verTodas.value ? vistas.value : vistas.value.slice(0, limite)))
const nivel = (s: number) => (s >= 60 ? 'forte' : s >= 40 ? 'média' : 'fraca')

// A busca leva ~7 s; a barra anda pelo tempo desde o início (o nmcli não informa progresso).
const agoraMs = ref(Date.now())
const pctBusca = computed(() => {
  const d = dados.value?.buscando_desde
  if (!d) return 5
  return Math.min(95, Math.round(((agoraMs.value - new Date(d).getTime()) / 7000) * 100))
})
let relogio: ReturnType<typeof setInterval> | undefined
watch(
  () => dados.value?.buscando,
  (b) => {
    clearInterval(relogio)
    if (b)
      relogio = setInterval(() => {
        agoraMs.value = Date.now()
        atualizar()
      }, 1000)
  },
)
onBeforeUnmount(() => clearInterval(relogio))

async function buscar() {
  aviso.value = null
  try {
    await pedir('POST', '/wifi/vizinhanca/buscar')
    await atualizar()
  } catch (e) {
    aviso.value = e instanceof ErroApi ? e.message : 'Não deu para buscar agora.'
  }
}

// Disputa usa as cores de estado do painel (com o texto ao lado, nunca só a cor).
const tomDisputa = { pouca: 'ok', média: 'alerta', muita: 'erro' } as const

function dicaCanal(c: FaixaWifi['canais'][number]) {
  const partes = [`Canal ${c.canal}`]
  if (c.proprio) partes.push('teu roteador')
  if (c.redes?.length) partes.push(c.redes.join(', '))
  else if (!c.proprio) partes.push('livre')
  if (c.dfs) partes.push('DFS: divide com radar')
  return partes.join(' · ')
}
</script>

<template>
  <CartaoIndicador titulo="Wi-Fi da vizinhança e melhor canal">
    <template #selo>
      <button class="botao" type="button" :disabled="!dados || dados.buscando" @click="buscar">
        {{ dados?.buscando ? 'Buscando…' : 'Buscar agora' }}
      </button>
    </template>

    <div v-if="dados?.buscando" class="ouvindo" aria-live="polite">
      <span class="suave">Ouvindo as redes por perto…</span>
      <div class="barra-busca"><span :style="{ width: `${pctBusca}%` }"></span></div>
    </div>
    <Carregando v-if="!dados" />
    <Indisponivel v-else-if="!dados.disponivel" :motivo="dados.motivo" />
    <template v-else-if="viz">
      <p v-if="aviso" class="aviso" role="status">{{ aviso }}</p>
      <p class="suave linha-topo">
        {{ vistas.length }} redes por perto · última busca {{ haQuantoTempo(viz.em) }} ({{ viz.redes.length }} ouvidas nela) ·
        {{ viz.buscas > 1 ? `recomendação pela média das últimas ${viz.buscas} buscas` : 'primeira busca: a recomendação firma com as próximas' }}
      </p>

      <div class="faixas">
        <section v-for="f in faixas" :key="f.nome" class="faixa" :aria-label="`Faixa ${f.nome}`">
          <header>
            <h3>{{ f.nome }}</h3>
            <SeloEstado v-if="f.canal_atual" :tom="f.mudar ? 'alerta' : 'ok'" :texto="f.mudar ? `Vale trocar para o ${f.recomendado}` : 'Canal bom'" />
          </header>
          <p v-if="f.tua_rede" class="tua">
            <strong>{{ f.tua_rede }}</strong> no canal <strong>{{ f.canal_atual }}</strong>
            <span class="suave"> · {{ f.vizinhos }} vizinho{{ f.vizinhos === 1 ? '' : 's' }} nesta faixa</span>
          </p>
          <p class="texto">{{ f.texto }}</p>

          <div class="barras" role="img" :aria-label="`Ocupação dos canais em ${f.nome}: altura = sinal do vizinho mais forte`">
            <div
              v-for="c in f.canais"
              :key="c.canal"
              class="coluna"
              :class="{ proprio: c.proprio, dfs: c.dfs }"
              :title="dicaCanal(c)"
            >
              <div class="trilho">
                <div class="barra" :style="{ height: `${c.mais_forte}%` }"></div>
              </div>
              <span class="num">{{ c.canal }}</span>
            </div>
          </div>
          <h4 class="sub-titulo">Comparando os canais</h4>
          <ol class="comparacao">
            <li v-for="c in f.comparacao" :key="c.rotulo" :class="{ melhor: c.melhor }">
              <span class="cand-topo">
                <strong>{{ c.rotulo }}</strong>
                <span class="disputa" :class="tomDisputa[c.disputa]">{{ c.disputa }} disputa</span>
                <span v-if="c.atual" class="marca">teu atual</span>
                <span v-if="c.melhor" class="marca melhor-marca">melhor</span>
              </span>
              <ul v-if="c.redes.length" class="vizinhos">
                <li v-for="r in c.redes" :key="`${r.nome}-${r.canal}`">
                  <span class="medidor" :title="`sinal ${r.sinal}%`"><span :class="r.nivel" :style="{ width: `${r.sinal}%` }"></span></span>
                  <span class="viz-nome">{{ r.nome }}</span>
                  <span class="viz-sinal">{{ r.sinal }}% · {{ r.nivel }}</span>
                </li>
              </ul>
              <span v-else class="motivo suave">livre de vizinhos</span>
              <span v-if="c.perto.length" class="motivo suave">
                + {{ c.perto.length }} em {{ c.perto.length === 1 ? 'canal vizinho que se sobrepõe' : 'canais vizinhos que se sobrepõem' }}
                ({{ c.perto.length > 3 ? 'os mais fortes: ' : '' }}{{ c.perto.slice(0, 3).map((r) => `${r.nome} ${r.sinal}%`).join(', ') }})
              </span>
            </li>
          </ol>

          <p class="legenda suave">
            Altura = sinal do vizinho mais forte no canal.
            <span class="amostra-tua" aria-hidden="true"></span> teu canal<template v-if="f.nome === '5 GHz'"> · canais esmaecidos são DFS</template>.
          </p>
        </section>
      </div>

      <h3 class="titulo-lista">Redes por perto <span class="suave">· {{ vistas.length }} nas últimas {{ viz.buscas }} busca{{ viz.buscas === 1 ? '' : 's' }}</span></h3>
      <ul class="redes">
        <li v-for="r in vistasMostradas" :key="`${r.ssid}-${r.canal}-${r.freq_mhz}`" :class="{ tua: r.propria || r.conectada }">
          <span class="medidor" :title="`sinal ${r.sinal_medio}%`"><span :class="nivel(r.sinal_medio)" :style="{ width: `${r.sinal_medio}%` }"></span></span>
          <span class="rede-nome">
            {{ r.ssid || 'rede oculta' }}
            <SeloEstado v-if="r.propria || r.conectada" tom="ok" texto="tua" />
          </span>
          <span class="rede-canal suave">{{ r.freq_mhz < 3000 ? '2,4' : '5' }} GHz · canal {{ r.canal }}</span>
          <span class="rede-sinal">{{ r.sinal_medio }}% <span class="suave">{{ nivel(r.sinal_medio) }}</span></span>
          <span class="rede-vezes suave" :title="'Em quantas das últimas buscas esta rede foi ouvida'">{{ r.vezes }}/{{ r.buscas }}</span>
        </li>
      </ul>
      <button v-if="vistas.length > limite" class="botao ver" type="button" @click="verTodas = !verTodas">
        {{ verTodas ? 'Mostrar menos' : `Ver todas as ${vistas.length}` }}
      </button>
      <p class="suave legenda-lista">
        Sinal = média das vezes em que a rede foi ouvida · “3/6” = ouvida em 3 das últimas 6 buscas (rede fraca ou
        longe às vezes não é ouvida). <span class="fraca-txt">WPA1</span> = proteção antiga (rede do vizinho).
      </p>
    </template>

    <template #rodape>
      Como compara: rede forte atrapalha muito mais que rede fraca, então um canal com várias redes fracas
      pode ser melhor que um com uma só forte. É uma estimativa: dá para ver o quão forte cada vizinho chega,
      mas não o quanto ele usa a internet.
      Nome, canal e sinal são anunciados por todo roteador; nada além disso é lido. Busca a cada 30 min
      (pausa o Wi-Fi deste PC por ~1 s). 5 GHz considera canais de 80 MHz. Trocar o canal é no painel do
      roteador.
    </template>
  </CartaoIndicador>
</template>

<style scoped>
.ouvindo {
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  margin-bottom: 0.75rem;
  font-size: 0.88rem;
}
.barra-busca {
  height: 0.45rem;
  border-radius: 999px;
  background: var(--superficie-2);
  overflow: hidden;
}
.barra-busca span {
  display: block;
  height: 100%;
  background: var(--primaria);
  transition: width 0.9s linear;
}
.aviso {
  margin: 0 0 0.5rem;
  color: var(--primaria);
  font-weight: 500;
}
.linha-topo {
  margin: 0 0 0.75rem;
  font-size: 0.85rem;
}
.faixas {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 22rem), 1fr));
  gap: 1.25rem;
}
.faixa header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
h3 {
  font-size: 1rem;
}
.tua {
  margin: 0.4rem 0 0;
  font-size: 0.9rem;
  overflow-wrap: anywhere;
}
.texto {
  margin: 0.3rem 0 0.75rem;
  font-size: 0.9rem;
}
.barras {
  display: flex;
  gap: 3px;
  align-items: flex-end;
  overflow-x: auto;
  padding-bottom: 2px;
}
.coluna {
  display: flex;
  flex-direction: column;
  align-items: center;
  flex: 1 0 1.1rem;
  min-width: 1.1rem;
}
.trilho {
  width: 100%;
  height: 5.5rem;
  display: flex;
  align-items: flex-end;
  border-bottom: 1px solid var(--borda);
}
.barra {
  width: 100%;
  min-height: 0;
  background: var(--serie-1);
  border-radius: 3px 3px 0 0;
}
.coluna.proprio .trilho {
  background: color-mix(in srgb, var(--serie-2) 14%, transparent);
  border-bottom: 3px solid var(--serie-2);
  border-radius: 3px 3px 0 0;
}
.coluna.dfs {
  opacity: 0.55;
}
.num {
  font-size: 0.68rem;
  color: var(--texto-suave);
  font-variant-numeric: tabular-nums;
  margin-top: 2px;
}
.coluna.proprio .num {
  color: var(--texto);
  font-weight: 700;
}
.legenda {
  margin: 0.35rem 0 0;
  font-size: 0.78rem;
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 0.3rem;
}
.amostra-tua {
  display: inline-block;
  width: 0.9rem;
  height: 0.6rem;
  border-bottom: 3px solid var(--serie-2);
  background: color-mix(in srgb, var(--serie-2) 14%, transparent);
}
.ver {
  margin-top: 0.6rem;
}
.titulo-lista {
  margin-top: 1.25rem;
  font-size: 0.95rem;
}
.titulo-lista .suave {
  font-weight: 400;
  font-size: 0.85rem;
}
.redes {
  list-style: none;
  margin: 0.5rem 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}
.redes li {
  display: grid;
  grid-template-columns: 4rem minmax(0, 1.4fr) minmax(0, 1fr) 6.5rem 2.5rem;
  gap: 0.6rem;
  align-items: center;
  padding: 0.35rem 0;
  border-bottom: 1px solid var(--borda);
  font-size: 0.88rem;
}
.redes li:last-child {
  border-bottom: 0;
}
.redes li.tua {
  font-weight: 600;
}
.rede-nome {
  display: flex;
  gap: 0.4rem;
  align-items: center;
  overflow-wrap: anywhere;
}
.rede-sinal,
.rede-vezes {
  font-variant-numeric: tabular-nums;
  text-align: right;
}
@media (max-width: 640px) {
  .redes li {
    grid-template-columns: 3rem minmax(0, 1fr) auto;
  }
  .rede-canal,
  .rede-vezes {
    grid-column: 2 / 4;
    text-align: left;
    font-size: 0.78rem;
  }
}
.legenda-lista {
  font-size: 0.78rem;
  margin: 0.5rem 0 0;
}
.fraca-txt {
  color: var(--alerta);
}
.sub-titulo {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--texto-suave);
  margin: 0.9rem 0 0.4rem;
}
.comparacao {
  margin: 0 0 0.5rem;
  padding-left: 1.2rem;
  display: flex;
  flex-direction: column;
  gap: 0.4rem;
  font-size: 0.88rem;
}
.comparacao li.melhor::marker {
  font-weight: 700;
}
.cand-topo {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 0.5rem;
  align-items: baseline;
}
.motivo {
  display: block;
  font-size: 0.82rem;
}
.vizinhos {
  list-style: none;
  margin: 0.3rem 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.2rem;
}
.vizinhos li {
  display: grid;
  grid-template-columns: 3.5rem minmax(0, 1fr) auto;
  align-items: center;
  gap: 0.5rem;
  font-size: 0.85rem;
}
.medidor {
  height: 0.45rem;
  border-radius: 999px;
  background: var(--superficie-2);
  border: 1px solid var(--borda);
  overflow: hidden;
}
.medidor span {
  display: block;
  height: 100%;
}
.medidor .forte {
  background: var(--erro);
}
.medidor .média {
  background: var(--alerta);
}
.medidor .fraca {
  background: var(--ok);
}
.viz-nome {
  overflow-wrap: anywhere;
}
.viz-sinal {
  color: var(--texto-suave);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}
.disputa {
  font-size: 0.78rem;
  font-weight: 600;
  padding: 0 0.45rem;
  border-radius: 999px;
}
.disputa.ok {
  color: var(--ok);
  background: var(--ok-fundo);
}
.disputa.alerta {
  color: var(--alerta);
  background: var(--alerta-fundo);
}
.disputa.erro {
  color: var(--erro);
  background: var(--erro-fundo);
}
.marca {
  font-size: 0.75rem;
  color: var(--texto-suave);
  border: 1px solid var(--borda);
  border-radius: 999px;
  padding: 0 0.45rem;
}
.melhor-marca {
  color: var(--texto);
  border-color: var(--texto-suave);
}
.fraca {
  color: var(--alerta);
}
</style>
