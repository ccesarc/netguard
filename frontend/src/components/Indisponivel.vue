<script setup lang="ts">
// Lugar de um dado que não existe. Nunca mostrar zero nem número de exemplo.
defineProps<{ motivo: string }>()
</script>

<template>
  <div class="indisponivel" role="note">
    <span class="traco" aria-hidden="true">—</span>
    <p><strong>Indisponível.</strong> {{ motivo }}</p>
  </div>
</template>

<style scoped>
.indisponivel {
  display: flex;
  gap: 0.6rem;
  align-items: flex-start;
  color: var(--texto-suave);
}
.traco {
  font-size: 1.6rem;
  line-height: 1;
  opacity: 0.6;
}
p {
  margin: 0;
  font-size: 0.9rem;
}
</style>
