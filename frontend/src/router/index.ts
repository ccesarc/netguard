import type { Component } from 'vue'
import { createRouter, createWebHistory, type RouteRecordRaw } from 'vue-router'
import { useSessao } from '../composables/useSessao'
import LayoutPainel from '../layouts/LayoutPainel.vue'
import VisaoGeral from '../views/VisaoGeral.vue'
import Entrar from '../views/Entrar.vue'
import EmBreve from '../views/EmBreve.vue'
import Internet from '../views/Internet.vue'
import Eventos from '../views/Eventos.vue'
import Configuracoes from '../views/Configuracoes.vue'
import Dispositivos from '../views/Dispositivos.vue'
import Saude from '../views/Saude.vue'
import { menu } from '../menu'

// Telas prontas; as outras mostram "chega na fase N".
const prontas: Record<string, Component> = {
  'visao-geral': VisaoGeral,
  internet: Internet,
  eventos: Eventos,
  configuracoes: Configuracoes,
  dispositivos: Dispositivos,
  saude: Saude,
}

const filhos: RouteRecordRaw[] = menu.map((m) =>
  m.nome in prontas
    ? { path: m.caminho.slice(1), name: m.nome, component: prontas[m.nome], meta: { titulo: m.titulo } }
    : {
        path: m.caminho.slice(1),
        name: m.nome,
        component: EmBreve,
        props: { titulo: m.titulo, fase: 'fase' in m ? m.fase : 0 },
        meta: { titulo: m.titulo },
      },
)

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/entrar', name: 'entrar', component: Entrar, meta: { publica: true, titulo: 'Entrar' } },
    { path: '/', component: LayoutPainel, children: filhos },
    { path: '/:resto(.*)*', redirect: '/' },
  ],
})

router.beforeEach(async (para) => {
  const { carregar } = useSessao()
  let usuario = null
  try {
    usuario = await carregar()
  } catch {
    // Backend fora do ar: deixa a tela abrir e mostrar o erro.
    return true
  }
  if (!para.meta.publica && !usuario) return { name: 'entrar', query: { de: para.fullPath } }
  if (para.name === 'entrar' && usuario) return { path: '/' }
  return true
})

router.afterEach((para) => {
  const t = para.meta.titulo
  document.title = typeof t === 'string' ? `${t} · NetGuard` : 'NetGuard'
})
