<script setup lang="ts">
defineProps<{ titulo: string }>()
</script>

<template>
  <section class="cartao">
    <header>
      <h2>{{ titulo }}</h2>
      <slot name="selo" />
    </header>
    <div class="corpo"><slot /></div>
    <footer v-if="$slots.rodape"><slot name="rodape" /></footer>
  </section>
</template>

<style scoped>
.cartao {
  background: var(--superficie);
  border: 1px solid var(--borda);
  border-radius: var(--raio);
  box-shadow: var(--sombra);
  padding: 1rem 1.1rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
  min-width: 0;
}
header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.5rem;
  flex-wrap: wrap;
}
h2 {
  font-size: 0.95rem;
  font-weight: 600;
  color: var(--texto-suave);
}
.corpo {
  flex: 1;
}
footer {
  font-size: 0.82rem;
  color: var(--texto-suave);
  border-top: 1px solid var(--borda);
  padding-top: 0.6rem;
}
</style>
