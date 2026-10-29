<script setup lang="ts">
withDefaults(defineProps<{ texto?: string }>(), { texto: 'Carregando…' })
</script>

<template>
  <div class="carregando" role="status" aria-live="polite">
    <span class="roda" aria-hidden="true"></span>{{ texto }}
  </div>
</template>

<style scoped>
.carregando {
  display: flex;
  align-items: center;
  gap: 0.6rem;
  color: var(--texto-suave);
  padding: 0.5rem 0;
}
.roda {
  width: 1rem;
  height: 1rem;
  border: 2px solid var(--borda);
  border-top-color: var(--primaria);
  border-radius: 50%;
  animation: girar 0.8s linear infinite;
}
@keyframes girar {
  to {
    transform: rotate(360deg);
  }
}
@media (prefers-reduced-motion: reduce) {
  .roda {
    animation: none;
  }
}
</style>
