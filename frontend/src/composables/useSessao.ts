import { ref } from 'vue'
import { pedir, ErroNaoAutenticado } from '../api/cliente'
import type { Usuario } from '../api/tipos'

const usuario = ref<Usuario | null>(null)
let conferida = false

export function useSessao() {
  // Pergunta ao backend uma vez; depois usa o que já sabe.
  async function carregar(): Promise<Usuario | null> {
    if (conferida) return usuario.value
    try {
      const r = await pedir<{ usuario: Usuario }>('GET', '/auth/eu')
      usuario.value = r.usuario
    } catch (e) {
      if (!(e instanceof ErroNaoAutenticado)) throw e
      usuario.value = null
    }
    conferida = true
    return usuario.value
  }

  async function entrar(nome: string, senha: string) {
    const r = await pedir<{ usuario: Usuario }>('POST', '/auth/entrar', { nome, senha })
    usuario.value = r.usuario
    conferida = true
  }

  async function sair() {
    try {
      await pedir('POST', '/auth/sair')
    } finally {
      esquecer()
    }
  }

  // Sessão caiu do lado do servidor (401 no meio do uso).
  function esquecer() {
    usuario.value = null
    conferida = true
  }

  return { usuario, carregar, entrar, sair, esquecer }
}
