<script setup lang="ts">
// Tela que ainda não tem dados. Diz quando chega em vez de mostrar exemplo.
defineProps<{ titulo: string; fase: number }>()

const oQueVem: Record<number, string> = {
  2: 'Alvos, intervalos e limites das verificações, teste de velocidade e qualidade do Wi-Fi.',
  3: 'Lista dos aparelhos observados na rede, com nome, fabricante, histórico e aviso de aparelho novo.',
  4: 'Integração com o Pi-hole, que vai rodar no Raspberry Pi.',
  5: 'Tráfego desta máquina, regras de alerta e avisos fora do painel.',
}
</script>

<template>
  <h1 class="titulo">{{ titulo }}</h1>
  <div class="aviso">
    <p><strong>Esta área ainda não tem dados.</strong> Ela chega na fase {{ fase }}.</p>
    <p v-if="oQueVem[fase]" class="suave">{{ oQueVem[fase] }}</p>
  </div>
</template>

<style scoped>
.titulo {
  font-size: 1.5rem;
  margin-bottom: 1rem;
}
.aviso {
  background: var(--superficie);
  border: 1px dashed var(--borda);
  border-radius: var(--raio);
  padding: 1rem 1.2rem;
  max-width: 44rem;
}
.aviso p {
  margin: 0.25rem 0;
}
</style>
