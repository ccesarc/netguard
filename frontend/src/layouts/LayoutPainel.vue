<script setup lang="ts">
import { ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { menu } from '../menu'
import { useSessao } from '../composables/useSessao'

const { usuario, sair } = useSessao()
const router = useRouter()
const rota = useRoute()
const menuAberto = ref(false)

// No celular o menu fecha ao trocar de tela.
watch(
  () => rota.fullPath,
  () => (menuAberto.value = false),
)

async function aoSair() {
  await sair().catch(() => {})
  router.push({ name: 'entrar' })
}
</script>

<template>
  <div class="painel" :class="{ 'menu-aberto': menuAberto }">
    <header class="topo-celular">
      <button
        class="alternar"
        type="button"
        :aria-expanded="menuAberto"
        aria-controls="menu-lateral"
        @click="menuAberto = !menuAberto"
      >
        <span aria-hidden="true">☰</span>
        <span class="so-leitor">Menu</span>
      </button>
      <span class="marca">NetGuard</span>
    </header>

    <nav id="menu-lateral" class="lateral" aria-label="Menu principal">
      <div class="marca marca-lateral">
        <img src="/favicon.svg" alt="" width="26" height="26" />
        NetGuard
      </div>
      <ul>
        <li v-for="item in menu" :key="item.nome">
          <RouterLink :to="item.caminho" class="link" :class="{ futuro: 'fase' in item }">
            <span class="icone" aria-hidden="true">{{ item.icone }}</span>
            {{ item.titulo }}
          </RouterLink>
        </li>
      </ul>
      <div class="rodape-menu">
        <span class="suave">{{ usuario?.nome }}</span>
        <button class="sair" type="button" @click="aoSair">Sair</button>
      </div>
    </nav>
    <div class="cortina" aria-hidden="true" @click="menuAberto = false"></div>

    <main class="conteudo">
      <RouterView />
    </main>
  </div>
</template>

<style scoped>
.painel {
  display: grid;
  grid-template-columns: 15rem minmax(0, 1fr);
  min-height: 100vh;
}
.lateral {
  background: var(--menu-fundo);
  color: var(--menu-texto);
  display: flex;
  flex-direction: column;
  padding: 1rem 0.75rem;
  position: sticky;
  top: 0;
  height: 100vh;
  overflow-y: auto;
}
.marca {
  font-weight: 700;
  letter-spacing: 0.01em;
}
.marca-lateral {
  display: flex;
  align-items: center;
  gap: 0.55rem;
  color: #fff;
  font-size: 1.1rem;
  padding: 0.25rem 0.5rem 1rem;
}
ul {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.link {
  display: flex;
  align-items: center;
  gap: 0.65rem;
  padding: 0.55rem 0.65rem;
  border-radius: 8px;
  color: inherit;
  text-decoration: none;
}
.link:hover {
  background: var(--menu-ativo);
  color: #fff;
}
.link.router-exact-active {
  background: var(--menu-ativo);
  color: #fff;
  box-shadow: inset 3px 0 0 var(--primaria);
}
.link.futuro {
  opacity: 0.7;
}
.icone {
  width: 1.2rem;
  text-align: center;
}
.rodape-menu {
  margin-top: auto;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0.75rem 0.5rem 0;
  border-top: 1px solid rgb(255 255 255 / 8%);
}
.sair {
  background: none;
  border: 1px solid rgb(255 255 255 / 18%);
  color: var(--menu-texto);
  border-radius: 6px;
  padding: 0.2rem 0.6rem;
}
.conteudo {
  padding: 1.5rem;
  min-width: 0;
}
.topo-celular,
.cortina {
  display: none;
}
.so-leitor {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}

@media (max-width: 860px) {
  .painel {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: auto 1fr; /* barra de cima não estica em página curta */
  }
  .topo-celular {
    display: flex;
    align-items: center;
    gap: 0.75rem;
    position: sticky;
    top: 0;
    z-index: 5;
    background: var(--menu-fundo);
    color: #fff;
    padding: 0.6rem 1rem;
  }
  .alternar {
    background: none;
    border: 0;
    color: inherit;
    font-size: 1.3rem;
    padding: 0.1rem 0.3rem;
  }
  .lateral {
    position: fixed;
    inset: 0 auto 0 0;
    width: min(17rem, 85vw);
    z-index: 20;
    transform: translateX(-100%);
    transition: transform 0.2s ease;
  }
  .menu-aberto .lateral {
    transform: none;
  }
  .menu-aberto .cortina {
    display: block;
    position: fixed;
    inset: 0;
    z-index: 10;
    background: rgb(0 0 0 / 45%);
  }
  .conteudo {
    padding: 1rem;
  }
}
</style>
