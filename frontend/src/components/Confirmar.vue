<script setup lang="ts">
// Diálogo de confirmação para ações que mudam algo (pausar bloqueio, apagar regra...).
import { ref, watch } from 'vue'

const props = withDefaults(
  defineProps<{ aberto: boolean; titulo: string; mensagem: string; textoConfirmar?: string; perigoso?: boolean }>(),
  { textoConfirmar: 'Confirmar', perigoso: false },
)
const emit = defineEmits<{ confirmar: []; cancelar: [] }>()
const dialogo = ref<HTMLDialogElement | null>(null)

watch(
  () => props.aberto,
  (aberto) => {
    const d = dialogo.value
    if (!d) return
    if (aberto && !d.open) d.showModal?.()
    if (!aberto && d.open) d.close?.()
  },
)
</script>

<template>
  <dialog ref="dialogo" class="dialogo" @cancel.prevent="emit('cancelar')">
    <h2>{{ titulo }}</h2>
    <p>{{ mensagem }}</p>
    <div class="acoes">
      <button class="botao" type="button" @click="emit('cancelar')">Cancelar</button>
      <button
        class="botao"
        :class="perigoso ? 'perigo' : 'botao-primario'"
        type="button"
        @click="emit('confirmar')"
      >
        {{ textoConfirmar }}
      </button>
    </div>
  </dialog>
</template>

<style scoped>
.dialogo {
  border: 1px solid var(--borda);
  border-radius: var(--raio);
  background: var(--superficie);
  color: var(--texto);
  padding: 1.25rem;
  width: min(28rem, calc(100vw - 2rem));
}
.dialogo::backdrop {
  background: rgb(0 0 0 / 45%);
}
h2 {
  font-size: 1.1rem;
}
.acoes {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
}
.perigo {
  background: var(--erro);
  border-color: var(--erro);
  color: #fff;
}
</style>
