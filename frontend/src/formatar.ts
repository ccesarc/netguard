const numero1 = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 1, minimumFractionDigits: 1 })
const inteiro = new Intl.NumberFormat('pt-BR', { maximumFractionDigits: 0 })

export function milissegundos(ms: number): string {
  return `${ms < 10 ? numero1.format(ms) : inteiro.format(ms)} ms`
}

export function porcentagem(fracao: number): string {
  const p = fracao * 100
  return `${Number.isInteger(p) ? inteiro.format(p) : numero1.format(p)}%`
}

// "agora", "há 8 s", "há 3 min", "há 2 h" ou a data.
export function haQuantoTempo(iso: string, agora = new Date()): string {
  const s = Math.round((agora.getTime() - new Date(iso).getTime()) / 1000)
  if (s < 3) return 'agora'
  if (s < 60) return `há ${s} s`
  if (s < 3600) return `há ${Math.floor(s / 60)} min`
  if (s < 86400) return `há ${Math.floor(s / 3600)} h`
  return new Date(iso).toLocaleString('pt-BR')
}

// "40 s", "3 min 20 s", "2 h 5 min", "1 dia 3 h"
export function duracao(segundos: number): string {
  const s = Math.round(segundos)
  if (s < 60) return `${s} s`
  if (s < 3600) {
    const r = s % 60
    return `${Math.floor(s / 60)} min${r ? ` ${r} s` : ''}`
  }
  if (s < 86400) {
    const m = Math.floor((s % 3600) / 60)
    return `${Math.floor(s / 3600)} h${m ? ` ${m} min` : ''}`
  }
  const d = Math.floor(s / 86400)
  const h = Math.floor((s % 86400) / 3600)
  return `${d} ${d === 1 ? 'dia' : 'dias'}${h ? ` ${h} h` : ''}`
}

export function dataHora(iso: string): string {
  return new Date(iso).toLocaleString('pt-BR', { day: '2-digit', month: '2-digit', hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

// 0.99876 -> "99,88%" (duas casas: a diferença entre 99,9% e 99,99% importa)
export function porcentagemDisponibilidade(fracao: number): string {
  return `${(fracao * 100).toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 })}%`
}

export function mbps(v: number): string {
  return `${v.toLocaleString('pt-BR', { maximumFractionDigits: v < 10 ? 1 : 0 })} Mbps`
}

export function megabytes(mb: number): string {
  return mb >= 1024 ? `${(mb / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 })} GB` : `${Math.round(mb)} MB`
}

// Sinal do NetworkManager (0 a 100) em palavras.
export function qualidadeSinal(s: number): { texto: string; tom: 'ok' | 'alerta' | 'erro' } {
  if (s >= 70) return { texto: 'Ótimo', tom: 'ok' }
  if (s >= 50) return { texto: 'Bom', tom: 'ok' }
  if (s >= 30) return { texto: 'Fraco', tom: 'alerta' }
  return { texto: 'Muito fraco', tom: 'erro' }
}
