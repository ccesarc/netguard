import { afterEach, describe, expect, it, vi } from 'vitest'
import { ErroApi, ErroNaoAutenticado, pedir } from '../api/cliente'

function respostaJSON(status: number, corpo: unknown) {
  return new Response(JSON.stringify(corpo), { status, headers: { 'Content-Type': 'application/json' } })
}

afterEach(() => vi.unstubAllGlobals())

describe('cliente da API', () => {
  it('manda o cabeçalho X-NetGuard e o corpo em JSON', async () => {
    const f = vi.fn().mockResolvedValue(respostaJSON(200, { ok: true }))
    vi.stubGlobal('fetch', f)
    await pedir('POST', '/auth/entrar', { nome: 'dono' })
    const [url, op] = f.mock.calls[0]
    expect(url).toBe('/api/v1/auth/entrar')
    expect(op.headers['X-NetGuard']).toBe('1')
    expect(op.headers['Content-Type']).toBe('application/json')
    expect(op.body).toBe('{"nome":"dono"}')
    expect(op.credentials).toBe('same-origin')
  })

  it('401 vira ErroNaoAutenticado com a mensagem do servidor', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaJSON(401, { erro: 'faça login' })))
    const p = pedir('GET', '/auth/eu')
    await expect(p).rejects.toBeInstanceOf(ErroNaoAutenticado)
    await expect(p).rejects.toThrow('faça login')
  })

  it('outros erros viram ErroApi com o status', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(respostaJSON(429, { erro: 'muitas tentativas' })))
    await expect(pedir('POST', '/auth/entrar', {})).rejects.toMatchObject({ status: 429, message: 'muitas tentativas' })
  })

  it('sem conexão dá mensagem clara', async () => {
    vi.stubGlobal('fetch', vi.fn().mockRejectedValue(new TypeError('Failed to fetch')))
    const p = pedir('GET', '/visao-geral')
    await expect(p).rejects.toBeInstanceOf(ErroApi)
    await expect(p).rejects.toThrow(/Sem conexão/)
  })

  it('204 devolve vazio', async () => {
    vi.stubGlobal('fetch', vi.fn().mockResolvedValue(new Response(null, { status: 204 })))
    await expect(pedir('POST', '/auth/sair')).resolves.toBeUndefined()
  })
})
