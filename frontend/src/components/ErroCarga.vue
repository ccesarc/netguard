<script setup lang="ts">
defineProps<{ mensagem: string }>()
defineEmits<{ tentar: [] }>()
</script>

<template>
  <div class="erro" role="alert">
    <p>{{ mensagem }}</p>
    <button class="botao" type="button" @click="$emit('tentar')">Tentar de novo</button>
  </div>
</template>

<style scoped>
.erro {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 1rem;
  flex-wrap: wrap;
  background: var(--erro-fundo);
  color: var(--erro);
  border-radius: var(--raio);
  padding: 0.75rem 1rem;
}
p {
  margin: 0;
  font-weight: 500;
}
</style>
