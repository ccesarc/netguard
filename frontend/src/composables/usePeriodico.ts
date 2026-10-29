import { onMounted, onUnmounted, ref, shallowRef } from 'vue'
import { ErroNaoAutenticado } from '../api/cliente'

// Busca agora e de novo a cada `intervaloMs`. Pausa com a aba escondida.
// Em 401 chama `naoAutenticado` (ir para o login) em vez de mostrar erro.
export function usePeriodico<T>(
  buscar: () => Promise<T>,
  intervaloMs: number,
  naoAutenticado?: () => void,
) {
  const dados = shallowRef<T | null>(null)
  const erro = ref<string | null>(null)
  const carregando = ref(true)
  let timer: ReturnType<typeof setTimeout> | undefined
  let ativo = true

  async function atualizar() {
    clearTimeout(timer)
    try {
      dados.value = await buscar()
      erro.value = null
    } catch (e) {
      if (e instanceof ErroNaoAutenticado) {
        naoAutenticado?.()
        return
      }
      erro.value = e instanceof Error ? e.message : String(e)
    } finally {
      carregando.value = false
    }
    if (ativo && !document.hidden) timer = setTimeout(atualizar, intervaloMs)
  }

  function aoMudarVisibilidade() {
    if (!document.hidden && ativo) atualizar()
    else clearTimeout(timer)
  }

  onMounted(() => {
    document.addEventListener('visibilitychange', aoMudarVisibilidade)
    atualizar()
  })
  onUnmounted(() => {
    ativo = false
    clearTimeout(timer)
    document.removeEventListener('visibilitychange', aoMudarVisibilidade)
  })

  return { dados, erro, carregando, atualizar }
}
