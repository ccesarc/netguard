<script setup lang="ts">
import type { Disponibilidade } from '../api/tipos'
import { duracao, porcentagem, porcentagemDisponibilidade } from '../formatar'
import Indisponivel from './Indisponivel.vue'

defineProps<{ titulo: string; d: Disponibilidade }>()
</script>

<template>
  <div class="disp">
    <h3>{{ titulo }}</h3>
    <Indisponivel v-if="!d.disponivel" :motivo="d.motivo" />
    <template v-else>
      <p class="numero" :class="{ ruim: d.percentual < 0.99 }">{{ porcentagemDisponibilidade(d.percentual) }}</p>
      <p class="linha">
        {{ d.quedas === 0 ? 'Nenhuma queda' : d.quedas === 1 ? '1 queda' : `${d.quedas} quedas` }}
        <span v-if="d.tempo_fora_s > 0"> · {{ duracao(d.tempo_fora_s) }} fora</span>
      </p>
      <p class="linha suave" :title="'Parte do período em que o monitor estava ligado medindo'">
        Medido em {{ d.cobertura > 0 && d.cobertura < 0.001 ? 'menos de 0,1%' : porcentagem(d.cobertura) }} do período
      </p>
    </template>
  </div>
</template>

<style scoped>
.disp {
  min-width: 0;
}
h3 {
  font-size: 0.85rem;
  font-weight: 600;
  color: var(--texto-suave);
  margin-bottom: 0.3rem;
}
.numero {
  margin: 0;
  font-size: 1.6rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.15;
}
.numero.ruim {
  color: var(--alerta);
}
.linha {
  margin: 0.15rem 0 0;
  font-size: 0.85rem;
}
</style>
