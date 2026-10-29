// Itens do menu lateral, na ordem pedida. `fase` = quando a tela ganha dados.
export const menu = [
  { caminho: '/', nome: 'visao-geral', titulo: 'Visão geral', icone: '◉' },
  { caminho: '/saude', nome: 'saude', titulo: 'Saúde', icone: '✚' },
  { caminho: '/internet', nome: 'internet', titulo: 'Internet', icone: '⇅' },
  { caminho: '/dispositivos', nome: 'dispositivos', titulo: 'Dispositivos', icone: '▣' },
  { caminho: '/consumo', nome: 'consumo', titulo: 'Consumo de rede', icone: '▤', fase: 5 },
  { caminho: '/dns', nome: 'dns', titulo: 'DNS e anúncios', icone: '⊘', fase: 4 },
  { caminho: '/alertas', nome: 'alertas', titulo: 'Alertas', icone: '△', fase: 5 },
  { caminho: '/eventos', nome: 'eventos', titulo: 'Histórico de eventos', icone: '☰' },
  { caminho: '/configuracoes', nome: 'configuracoes', titulo: 'Configurações', icone: '⚙' },
] as const
