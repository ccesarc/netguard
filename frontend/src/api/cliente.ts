// Cliente da API. Toda chamada leva o cabeçalho X-NetGuard (o backend recusa
// escrita sem ele) e o cookie de sessão, que o navegador manda sozinho.

export class ErroApi extends Error {
  constructor(
    public status: number,
    mensagem: string,
    public detalhes: string[] = [],
  ) {
    super(mensagem)
    this.name = 'ErroApi'
  }
}

export class ErroNaoAutenticado extends ErroApi {
  constructor(mensagem = 'Faça login') {
    super(401, mensagem)
    this.name = 'ErroNaoAutenticado'
  }
}

type Metodo = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export async function pedir<T>(metodo: Metodo, caminho: string, corpo?: unknown): Promise<T> {
  const cabecalhos: Record<string, string> = { 'X-NetGuard': '1', Accept: 'application/json' }
  if (corpo !== undefined) cabecalhos['Content-Type'] = 'application/json'

  let resp: Response
  try {
    resp = await fetch(`/api/v1${caminho}`, {
      method: metodo,
      headers: cabecalhos,
      credentials: 'same-origin',
      body: corpo === undefined ? undefined : JSON.stringify(corpo),
    })
  } catch {
    throw new ErroApi(0, 'Sem conexão com o NetGuard. O serviço está rodando?')
  }

  if (resp.status === 204) return undefined as T
  let dados: unknown = null
  try {
    dados = await resp.json()
  } catch {
    // resposta sem JSON (ex.: proxy do Vite sem backend)
  }
  if (!resp.ok) {
    const msg =
      dados && typeof dados === 'object' && 'erro' in dados && typeof dados.erro === 'string'
        ? dados.erro
        : `Erro ${resp.status}`
    if (resp.status === 401) throw new ErroNaoAutenticado(msg)
    const detalhes =
      dados && typeof dados === 'object' && 'erros' in dados && Array.isArray(dados.erros)
        ? dados.erros.filter((x): x is string => typeof x === 'string')
        : []
    throw new ErroApi(resp.status, msg, detalhes)
  }
  return dados as T
}
