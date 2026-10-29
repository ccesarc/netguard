<script setup lang="ts">
export type Tom = 'ok' | 'alerta' | 'erro' | 'neutro'
defineProps<{ tom: Tom; texto: string }>()
</script>

<template>
  <span class="selo" :class="tom"><span class="ponto" aria-hidden="true"></span>{{ texto }}</span>
</template>

<style scoped>
.selo {
  display: inline-flex;
  align-items: center;
  gap: 0.4rem;
  padding: 0.15rem 0.6rem;
  border-radius: 999px;
  font-size: 0.82rem;
  font-weight: 600;
  white-space: nowrap;
}
.ponto {
  width: 0.5rem;
  height: 0.5rem;
  border-radius: 50%;
  background: currentColor;
}
.ok {
  color: var(--ok);
  background: var(--ok-fundo);
}
.alerta {
  color: var(--alerta);
  background: var(--alerta-fundo);
}
.erro {
  color: var(--erro);
  background: var(--erro-fundo);
}
.neutro {
  color: var(--neutro);
  background: var(--neutro-fundo);
}
</style>
