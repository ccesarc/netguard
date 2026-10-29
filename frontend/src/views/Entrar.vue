<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { useSessao } from '../composables/useSessao'

const { entrar } = useSessao()
const router = useRouter()
const rota = useRoute()

const nome = ref('')
const senha = ref('')
const erro = ref<string | null>(null)
const enviando = ref(false)

async function enviar() {
  erro.value = null
  enviando.value = true
  try {
    await entrar(nome.value.trim(), senha.value)
    const de = typeof rota.query.de === 'string' && rota.query.de.startsWith('/') ? rota.query.de : '/'
    router.replace(de)
  } catch (e) {
    erro.value = e instanceof Error ? e.message : 'Não foi possível entrar'
    senha.value = ''
  } finally {
    enviando.value = false
  }
}
</script>

<template>
  <main class="pagina">
    <form class="caixa" @submit.prevent="enviar">
      <div class="marca">
        <img src="/favicon.svg" alt="" width="36" height="36" />
        <div>
          <h1>NetGuard</h1>
          <p class="suave">Monitor da rede de casa</p>
        </div>
      </div>

      <label>
        Usuário
        <input v-model="nome" name="usuario" autocomplete="username" required maxlength="32" autofocus />
      </label>
      <label>
        Senha
        <input v-model="senha" name="senha" type="password" autocomplete="current-password" required maxlength="128" />
      </label>

      <p v-if="erro" class="erro" role="alert">{{ erro }}</p>

      <button class="botao botao-primario entrar" type="submit" :disabled="enviando">
        {{ enviando ? 'Entrando…' : 'Entrar' }}
      </button>
      <p class="dica suave">Usuário criado no terminal: <span class="mono">netguardd usuario criar &lt;nome&gt;</span></p>
    </form>
  </main>
</template>

<style scoped>
.pagina {
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 1rem;
}
.caixa {
  width: min(23rem, 100%);
  background: var(--superficie);
  border: 1px solid var(--borda);
  border-radius: var(--raio);
  box-shadow: var(--sombra);
  padding: 1.5rem;
  display: flex;
  flex-direction: column;
  gap: 1rem;
}
.marca {
  display: flex;
  align-items: center;
  gap: 0.75rem;
}
h1 {
  font-size: 1.35rem;
}
.marca p {
  margin: 0;
  font-size: 0.88rem;
}
label {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  font-weight: 500;
  font-size: 0.9rem;
}
input {
  font: inherit;
  padding: 0.55rem 0.7rem;
  border: 1px solid var(--borda);
  border-radius: 8px;
  background: var(--superficie-2);
  color: var(--texto);
}
.erro {
  margin: 0;
  color: var(--erro);
  background: var(--erro-fundo);
  border-radius: 8px;
  padding: 0.5rem 0.75rem;
  font-size: 0.9rem;
}
.entrar {
  justify-content: center;
  padding: 0.65rem;
}
.dica {
  margin: 0;
  font-size: 0.78rem;
}
</style>
