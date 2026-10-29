import { describe, expect, it } from 'vitest'
import { duracao, haQuantoTempo, milissegundos, porcentagem, porcentagemDisponibilidade } from '../formatar'

describe('formatar', () => {
  it('milissegundos com uma casa abaixo de 10', () => {
    expect(milissegundos(6.84)).toBe('6,8 ms')
    expect(milissegundos(48.7)).toBe('49 ms')
  })

  it('porcentagem', () => {
    expect(porcentagem(0)).toBe('0%')
    expect(porcentagem(0.2)).toBe('20%')
    expect(porcentagem(1 / 3)).toBe('33,3%')
  })

  it('há quanto tempo', () => {
    const agora = new Date('2026-10-29T12:00:00Z')
    expect(haQuantoTempo('2026-10-29T12:00:00Z', agora)).toBe('agora')
    expect(haQuantoTempo('2026-10-29T11:59:52Z', agora)).toBe('há 8 s')
    expect(haQuantoTempo('2026-10-29T11:57:00Z', agora)).toBe('há 3 min')
    expect(haQuantoTempo('2026-10-29T10:00:00Z', agora)).toBe('há 2 h')
  })
})

describe('duração e disponibilidade', () => {
  it('duração', () => {
    expect(duracao(40)).toBe('40 s')
    expect(duracao(95)).toBe('1 min 35 s')
    expect(duracao(120)).toBe('2 min')
    expect(duracao(7500)).toBe('2 h 5 min')
    expect(duracao(97200)).toBe('1 dia 3 h')
  })
  it('disponibilidade com duas casas', () => {
    expect(porcentagemDisponibilidade(0.99876)).toBe('99,88%')
    expect(porcentagemDisponibilidade(1)).toBe('100,00%')
  })
})
