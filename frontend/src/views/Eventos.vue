<script setup lang="ts">
import { pedir } from '../api/cliente'
import type { Dispositivo, Queda } from '../api/tipos'
import { dataHora } from '../formatar'
import SeloEstado from '../components/SeloEstado.vue'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import ListaQuedas from '../components/ListaQuedas.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

const { dados, erro, atualizar } = usePeriodico(
  () => pedir<{ quedas: Queda[] }>('GET', '/quedas?periodo=30d&limite=200'),
  30_000,
  useSessaoExpirada(),
)
const novos = usePeriodico(() => pedir<{ dispositivos: Dispositivo[] }>('GET', '/aparelhos-novos?periodo=30d'), 60_000, useSessaoExpirada())
</script>

<template>
  <h1 class="titulo">Histórico de eventos</h1>
  <ErroCarga v-if="erro" :mensagem="erro" class="bloco" @tentar="atualizar" />
  <CartaoIndicador titulo="Quedas da internet — últimos 30 dias">
    <ListaQuedas v-if="dados" :quedas="dados.quedas" />
    <Carregando v-else />
    <template #rodape>
      Uma queda só é registrada depois de 3 verificações seguidas sem internet (cerca de 30 s), para
      um pacote perdido não virar queda.
    </template>
  </CartaoIndicador>

  <CartaoIndicador titulo="Aparelhos que apareceram — últimos 30 dias" class="topo">
    <Carregando v-if="!novos.dados.value" />
    <p v-else-if="novos.dados.value.dispositivos.length === 0" class="suave vazio">Nenhum aparelho apareceu pela primeira vez neste período.</p>
    <ul v-else class="lista">
      <li v-for="d in novos.dados.value.dispositivos" :key="d.id">
        <span>
          <RouterLink to="/dispositivos">{{ d.nome }}</RouterLink>
          <span class="mono suave"> {{ d.ip }}</span>
        </span>
        <span class="lado">
          <SeloEstado v-if="d.alertar_novo" tom="alerta" texto="Novo" />
          <SeloEstado v-else-if="d.confiavel" tom="ok" texto="Conhecido" />
          <span class="suave">{{ dataHora(d.primeiro_visto) }}</span>
        </span>
      </li>
    </ul>
    <template #rodape>Os aparelhos da primeira procura de todas contam como o retrato inicial da casa, sem aviso.</template>
  </CartaoIndicador>
</template>

<style scoped>
.titulo {
  font-size: 1.5rem;
  margin-bottom: 1rem;
}
.bloco {
  margin-bottom: 1rem;
}
.topo {
  margin-top: 1rem;
}
.vazio {
  margin: 0;
}
.lista {
  list-style: none;
  margin: 0;
  padding: 0;
}
.lista li {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
  padding: 0.45rem 0;
  border-bottom: 1px solid var(--borda);
}
.lista li:last-child {
  border-bottom: 0;
}
.lado {
  display: flex;
  gap: 0.5rem;
  align-items: center;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
}
a {
  color: var(--primaria);
  text-decoration: none;
}
</style>
