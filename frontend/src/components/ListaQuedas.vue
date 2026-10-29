<script setup lang="ts">
import type { Queda } from '../api/tipos'
import { dataHora, duracao } from '../formatar'
import SeloEstado from './SeloEstado.vue'

defineProps<{ quedas: Queda[] }>()
</script>

<template>
  <p v-if="quedas.length === 0" class="vazio suave">Nenhuma queda registrada neste período.</p>
  <ul v-else class="lista">
    <li v-for="q in quedas" :key="q.id" class="item">
      <div class="topo">
        <SeloEstado :tom="q.em_andamento ? 'erro' : 'neutro'" :texto="q.em_andamento ? 'Em andamento' : 'Resolvida'" />
        <strong class="duracao">{{ duracao(q.duracao_s) }}</strong>
      </div>
      <p class="causa">{{ q.causa.texto }}</p>
      <p class="quando suave">
        {{ dataHora(q.inicio) }}
        <template v-if="q.fim"> → {{ dataHora(q.fim) }}</template>
      </p>
      <p v-if="q.causas.length > 1" class="detalhe suave">
        Causas ao longo da queda: {{ q.causas.map((c) => c.texto).join(' → ') }}
      </p>
      <p v-if="q.detalhe" class="detalhe suave">{{ q.detalhe }}</p>
    </li>
  </ul>
</template>

<style scoped>
.vazio {
  margin: 0;
}
.lista {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
}
.item {
  padding: 0.75rem 0;
  border-bottom: 1px solid var(--borda);
}
.item:first-child {
  padding-top: 0;
}
.item:last-child {
  border-bottom: 0;
  padding-bottom: 0;
}
.topo {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
}
.duracao {
  font-variant-numeric: tabular-nums;
}
.causa {
  margin: 0.35rem 0 0;
  font-weight: 500;
}
.quando,
.detalhe {
  margin: 0.15rem 0 0;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
</style>
