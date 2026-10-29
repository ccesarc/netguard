import { useRouter } from 'vue-router'
import { useSessao } from './useSessao'

// Devolve a função que leva ao login quando a API responde 401.
export function useSessaoExpirada() {
  const router = useRouter()
  const { esquecer } = useSessao()
  return () => {
    esquecer()
    router.push({ name: 'entrar', query: { de: router.currentRoute.value.fullPath } })
  }
}
