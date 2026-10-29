<script setup lang="ts">
import { computed, watch } from 'vue'
import { pedir } from '../api/cliente'
import type { Periodo, Wifi } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { qualidadeSinal } from '../formatar'
import CartaoIndicador from './CartaoIndicador.vue'
import GraficoSerie from './GraficoSerie.vue'
import Indisponivel from './Indisponivel.vue'
import Carregando from './Carregando.vue'
import SeloEstado from './SeloEstado.vue'

const props = defineProps<{ periodo: Periodo }>()
const { dados, atualizar } = usePeriodico(() => pedir<Wifi>('GET', `/wifi?periodo=${props.periodo}`), 60_000, useSessaoExpirada())
watch(() => props.periodo, atualizar)

const atual = computed(() => (dados.value?.atual.disponivel ? dados.value.atual.amostra : null))
const qualidade = computed(() => (atual.value ? qualidadeSinal(atual.value.sinal) : null))
const temPontos = computed(() => !!dados.value && dados.value.historico.sinal.some((v) => v !== null))
const faixa = computed(() => {
  const f = atual.value?.freq_mhz ?? 0
  return f >= 5900 ? '6 GHz' : f >= 4900 ? '5 GHz' : f > 0 ? '2,4 GHz' : ''
})
</script>

<template>
  <CartaoIndicador titulo="Wi-Fi deste aparelho">
    <template v-if="qualidade" #selo><SeloEstado :tom="qualidade.tom" :texto="`Sinal ${qualidade.texto.toLowerCase()}`" /></template>
    <Carregando v-if="!dados" />
    <Indisponivel v-else-if="!dados.atual.disponivel" :motivo="dados.atual.motivo" />
    <template v-else-if="atual">
      <dl class="dados">
        <div>
          <dt>Rede</dt>
          <dd>{{ atual.ssid }}</dd>
        </div>
        <div>
          <dt>Sinal</dt>
          <dd>{{ atual.sinal }}%</dd>
        </div>
        <div>
          <dt>Faixa e canal</dt>
          <dd>{{ faixa }} · canal {{ atual.canal }}</dd>
        </div>
        <div>
          <dt>Ligação com o roteador</dt>
          <dd>{{ atual.taxa_mbps }} Mbps</dd>
        </div>
        <div>
          <dt>Outras redes no mesmo canal</dt>
          <dd>{{ atual.redes_mesmo_canal }} <span class="suave">de {{ atual.redes_vistas }} vistas</span></dd>
        </div>
      </dl>
      <p v-if="atual.redes_mesmo_canal >= 3" class="dica">
        Muitas redes no mesmo canal disputam o ar: trocar o canal no roteador pode ajudar.
      </p>
    </template>

    <GraficoSerie
      v-if="dados && temPontos"
      class="grafico"
      :t="dados.historico.t"
      :series="[
        { nome: 'Sinal médio', valores: dados.historico.sinal, cor: 1 },
        { nome: 'Pior momento', valores: dados.historico.sinal_min, cor: 3 },
      ]"
      :passo-s="dados.historico.passo_s"
      unidade="sinal"
      :altura="160"
      rotulo="Força do sinal do Wi-Fi no período"
    />
    <template #rodape>
      “Ligação com o roteador” é a velocidade do Wi-Fi até o roteador, não da internet. Sinal pelo
      NetworkManager, lido a cada minuto.
    </template>
  </CartaoIndicador>
</template>

<style scoped>
.dados {
  margin: 0;
  display: grid;
  gap: 0.35rem;
}
.dados div {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  font-size: 0.9rem;
}
dt {
  color: var(--texto-suave);
}
dd {
  margin: 0;
  text-align: right;
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
}
.dica {
  margin: 0.6rem 0 0;
  font-size: 0.85rem;
  color: var(--alerta);
}
.grafico {
  margin-top: 0.75rem;
}
</style>
