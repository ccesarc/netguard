<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { ErroApi, ErroNaoAutenticado, pedir } from '../api/cliente'
import type { Aparicao, Configuracao, Dispositivo, ListaDispositivos, Movimento, ResumoPresenca } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { dataHora, duracao, haQuantoTempo } from '../formatar'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import Confirmar from '../components/Confirmar.vue'
import SeloEstado from '../components/SeloEstado.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

const expirou = useSessaoExpirada()
const { dados, erro, atualizar } = usePeriodico(() => pedir<ListaDispositivos>('GET', '/dispositivos'), 30_000, expirou)
const mov = usePeriodico(() => pedir<{ movimentos: Movimento[]; janela_min: number }>('GET', '/dispositivos/movimentos?periodo=24h'), 30_000, expirou)

// ---- procura ao vivo ----
// Enquanto a procura roda, consulta a cada 1,5 s; quem aparece (ou volta) ganha destaque.
const recem = ref(new Set<number>())
const resumoFim = ref<string | null>(null)
let antes: Map<number, boolean> | null = null
let inicioProcura = 0
let rapido: ReturnType<typeof setTimeout> | undefined

function acompanhar() {
  clearTimeout(rapido)
  rapido = setTimeout(async () => {
    await Promise.all([atualizar(), mov.atualizar()])
    if (dados.value?.estado.varrendo) acompanhar()
  }, 1500)
}

watch(dados, (d, velho) => {
  if (!d) return
  if (d.estado.varrendo && !antes) {
    antes = new Map((velho ?? d).dispositivos.map((x) => [x.id, x.na_rede]))
    inicioProcura = Date.now()
    recem.value = new Set()
    resumoFim.value = null
    acompanhar()
  }
  if (antes) {
    for (const x of d.dispositivos) {
      if (x.na_rede && !antes.get(x.id)) recem.value.add(x.id)
    }
  }
  if (!d.estado.varrendo && antes) {
    const s = Math.round((Date.now() - inicioProcura) / 1000)
    const n = recem.value.size
    resumoFim.value = `Procura concluída em ${s} s: ${d.resumo.na_rede} na rede${n ? ` · ${n} apareceu${n > 1 ? 'ram' : ''} agora` : ' · ninguém novo'}.`
    antes = null
  }
})

onBeforeUnmount(() => clearTimeout(rapido))

const progresso = computed(() => dados.value?.estado.progresso ?? null)
const pctProcura = computed(() => (progresso.value ? Math.round((progresso.value.feitos / Math.max(1, progresso.value.total)) * 100) : 0))
// Situação de verdade: "na rede" vale 10 min para a lista não piscar, mas quem
// não respondeu à última procura (ou some há mais de 6 min) já aparece como "talvez saiu".
type Situacao = 'agora' | 'sumiu' | 'fora'
function situacao(d: Dispositivo): Situacao {
  if (!d.na_rede) return 'fora'
  if (d.este_aparelho) return 'agora'
  const visto = new Date(d.ultimo_visto).getTime()
  const inicio = dados.value?.estado.ultima_varredura_inicio
  const naoRespondeu = !!inicio && !dados.value?.estado.varrendo && visto < new Date(inicio).getTime()
  if (naoRespondeu || Date.now() - visto > 6 * 60_000) return 'sumiu'
  return 'agora'
}
const respondendo = computed(() => (dados.value?.dispositivos ?? []).filter((d) => situacao(d) === 'agora').length)

const hora = (iso: string) => new Date(iso).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' })

type Filtro = 'todos' | 'na_rede' | 'novos'
const filtro = ref<Filtro>('todos')
const busca = ref('')
const aviso = ref<string | null>(null)
const confirmarVarredura = ref<'ligar' | 'agora' | null>(null)

const lista = computed(() => {
  const q = busca.value.trim().toLowerCase()
  return (dados.value?.dispositivos ?? []).filter((d) => {
    if (filtro.value === 'na_rede' && !d.na_rede) return false
    if (filtro.value === 'novos' && !d.alertar_novo) return false
    if (!q) return true
    return [d.nome, d.ip, d.mac, d.fabricante, d.nome_descoberto].some((c) => c.toLowerCase().includes(q))
  })
})
const novos = computed(() => (dados.value?.dispositivos ?? []).filter((d) => d.alertar_novo))

// ---- detalhe ----
const aberto = ref<Dispositivo | null>(null)
const aparicoes = ref<Aparicao[] | null>(null)
const resumo7d = ref<ResumoPresenca | null>(null)
const apelido = ref('')
const notas = ref('')
const salvando = ref(false)
const erroDetalhe = ref<string | null>(null)

async function abrir(d: Dispositivo) {
  aberto.value = d
  apelido.value = d.apelido
  notas.value = d.notas
  aparicoes.value = null
  resumo7d.value = null
  erroDetalhe.value = null
  try {
    const r = await pedir<{ aparicoes: Aparicao[]; resumo_7d: ResumoPresenca }>('GET', `/dispositivos/${d.id}`)
    aparicoes.value = r.aparicoes
    resumo7d.value = r.resumo_7d
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    erroDetalhe.value = e instanceof Error ? e.message : String(e)
  }
}

async function alterar(id: number, corpo: Record<string, unknown>) {
  salvando.value = true
  erroDetalhe.value = null
  try {
    const r = await pedir<{ dispositivo: Dispositivo }>('PATCH', `/dispositivos/${id}`, corpo)
    if (aberto.value?.id === id) aberto.value = r.dispositivo
    await atualizar()
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    erroDetalhe.value = e instanceof ErroApi ? e.message : 'Não deu para salvar.'
  } finally {
    salvando.value = false
  }
}

const salvarDetalhe = () => aberto.value && alterar(aberto.value.id, { apelido: apelido.value, notas: notas.value })

function abrirPorId(id: number) {
  const d = dados.value?.dispositivos.find((x) => x.id === id)
  if (d) abrir(d)
}

// ---- varredura ----
async function ligarVarredura() {
  confirmarVarredura.value = null
  aviso.value = null
  try {
    const c = await pedir<Configuracao>('GET', '/configuracao')
    await pedir('PUT', '/configuracao', { ...c.ajustes, varredura_ativa: true })
    await pedir('POST', '/dispositivos/varrer').catch(() => {})
    await atualizar()
    aviso.value = 'Procura automática ligada: a cada 5 minutos, sozinha.'
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    aviso.value = e instanceof ErroApi ? e.message : 'Não deu para ligar a varredura.'
  }
}

async function varrerAgora() {
  confirmarVarredura.value = null
  aviso.value = null
  try {
    await pedir('POST', '/dispositivos/varrer')
    await atualizar()
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    aviso.value = e instanceof ErroApi ? e.message : 'Não deu para varrer agora.'
  }
}

function confirmarAcao() {
  if (confirmarVarredura.value === 'ligar') ligarVarredura()
  else varrerAgora()
}

// "14× em 7 dias · 31 h" (horas arredondadas; menos de 1 h vira minutos)
function presenca(d: Dispositivo): string {
  const p = d.presenca_7d
  if (!p || d.este_aparelho) return ''
  const tempo = p.segundos >= 3600 ? `${Math.round(p.segundos / 3600)} h` : `${Math.max(1, Math.round(p.segundos / 60))} min`
  return p.entradas ? `${p.entradas}× em 7 dias · ${tempo}` : `na rede há mais de 7 dias · ${tempo}`
}

function tipoMac(d: Dispositivo) {
  return d.mac_aleatorio ? 'MAC aleatório (privacidade)' : d.fabricante || 'fabricante desconhecido'
}
</script>

<template>
  <div class="cabecalho">
    <h1>Dispositivos</h1>
    <button
      v-if="dados"
      class="botao"
      type="button"
      :disabled="dados.estado.varrendo"
      @click="confirmarVarredura = 'agora'"
    >
      {{ dados.estado.varrendo ? 'Procurando…' : 'Procurar agora' }}
    </button>
  </div>

  <ErroCarga v-if="erro" :mensagem="erro" class="bloco" @tentar="atualizar" />
  <Carregando v-if="!dados && !erro" />

  <template v-if="dados">
    <p v-if="aviso" class="aviso bloco" role="status">{{ aviso }}</p>

    <section v-if="dados.estado.varrendo" class="procura bloco" aria-live="polite">
      <div class="radar" aria-hidden="true"><span></span></div>
      <div class="procura-texto">
        <strong>Procurando aparelhos na rede…</strong>
        <div class="barra-procura" role="progressbar" :aria-valuenow="pctProcura" aria-valuemin="0" aria-valuemax="100">
          <span :style="{ width: `${pctProcura}%` }"></span>
        </div>
        <span class="suave">
          <template v-if="progresso">{{ progresso.feitos }} de {{ progresso.total }} endereços · {{ progresso.achados }} aparelhos responderam</template>
          <template v-else>começando…</template>
          <template v-if="recem.size"> · {{ recem.size }} apareceu{{ recem.size > 1 ? 'ram' : '' }} agora</template>
        </span>
      </div>
    </section>
    <p v-else-if="resumoFim" class="concluida bloco" role="status">✔ {{ resumoFim }}</p>

    <div v-if="!dados.estado.varredura_ativa" class="faixa bloco">
      <div>
        <strong>A procura automática está desligada.</strong>
        Só aparecem os aparelhos que conversaram com este PC há pouco. Ligada, o NetGuard manda um ping
        para cada endereço da rede a cada 5 minutos (nada além disso) e acha quase todos.
      </div>
      <button class="botao botao-primario" type="button" @click="confirmarVarredura = 'ligar'">Ligar procura</button>
    </div>

    <div v-if="novos.length" class="novos bloco" role="alert">
      <strong>{{ novos.length === 1 ? 'Um aparelho novo apareceu na rede' : `${novos.length} aparelhos novos apareceram na rede` }}</strong>
      <ul>
        <li v-for="d in novos" :key="d.id">
          <span>{{ d.nome }} <span class="mono suave">{{ d.ip }}</span> · desde {{ dataHora(d.primeiro_visto) }}</span>
          <span class="acoes-novo">
            <button class="botao" type="button" :disabled="salvando" @click="alterar(d.id, { confiavel: true })">É conhecido</button>
            <button class="botao" type="button" :disabled="salvando" @click="alterar(d.id, { visto_novo: true })">Já vi</button>
            <button class="botao" type="button" @click="abrir(d)">Detalhes</button>
          </span>
        </li>
      </ul>
    </div>

    <div class="resumo bloco">
      <div><strong>{{ respondendo }}</strong> na rede agora</div>
      <div><strong>{{ dados.resumo.total }}</strong> já vistos</div>
      <div v-if="dados.estado.ultima_varredura" class="suave">Última procura {{ haQuantoTempo(dados.estado.ultima_varredura) }}</div>
    </div>

    <CartaoIndicador titulo="Aparelhos observados">
      <template #selo>
        <div class="filtros" role="group" aria-label="Filtro">
          <button v-for="f in ([['todos', 'Todos'], ['na_rede', 'Na rede'], ['novos', 'Novos']] as const)" :key="f[0]" type="button"
            class="filtro" :class="{ ativo: filtro === f[0] }" :aria-pressed="filtro === f[0]" @click="filtro = f[0]">
            {{ f[1] }}
          </button>
        </div>
      </template>
      <input v-model="busca" class="busca" type="search" placeholder="Procurar por nome, IP, MAC ou fabricante" aria-label="Procurar aparelho" />
      <p v-if="lista.length === 0" class="suave vazio">Nenhum aparelho com esse filtro.</p>
      <ul v-else class="lista">
        <li v-for="d in lista" :key="d.id">
          <button type="button" class="linha" :class="{ fora: situacao(d) === 'fora', chegou: recem.has(d.id) }" @click="abrir(d)">
            <span class="ponto" :class="situacao(d)" :title="{ agora: 'Na rede', sumiu: 'Não respondeu à última procura', fora: 'Fora da rede' }[situacao(d)]"></span>
            <span class="principal">
              <span class="nome">
                {{ d.nome }}
                <span v-if="recem.has(d.id)" class="acabou">acabou de aparecer</span>
                <SeloEstado v-if="d.alertar_novo" tom="alerta" texto="Novo" />
                <SeloEstado v-else-if="d.confiavel" tom="ok" texto="Conhecido" />
              </span>
              <span class="sub suave">{{ tipoMac(d) }}<template v-if="presenca(d)"> · {{ presenca(d) }}</template></span>
            </span>
            <span class="tecnico">
              <span class="mono">{{ d.ip }}</span>
              <span v-if="situacao(d) === 'agora'" class="suave">agora</span>
              <span v-else-if="situacao(d) === 'sumiu'" class="sumiu-texto">não respondeu · visto {{ haQuantoTempo(d.ultimo_visto) }}</span>
              <span v-else class="suave">{{ haQuantoTempo(d.ultimo_visto) }}</span>
            </span>
          </button>
        </li>
      </ul>
      <template #rodape>
        Aparelhos <strong>observados</strong>, não necessariamente todos: aparelho dormindo ou numa rede de
        visitantes isolada pode não aparecer. Nome vem do roteador; celulares com MAC aleatório não mostram fabricante.
      </template>
    </CartaoIndicador>
  </template>

  <CartaoIndicador v-if="dados" titulo="Entrou e saiu — últimas 24 h" class="topo-mov">
    <p v-if="!mov.dados.value" class="suave">Carregando…</p>
    <p v-else-if="mov.dados.value.movimentos.length === 0" class="suave">Ninguém entrou nem saiu nas últimas 24 h.</p>
    <ul v-else class="movimentos">
      <li v-for="m in mov.dados.value.movimentos.slice(0, 40)" :key="`${m.tipo}-${m.dispositivo_id}-${m.em}`">
        <span class="mov-hora">{{ hora(m.em) }}</span>
        <template v-if="m.tipo === 'desligado'">
          <span class="mov-tipo desligado">◌ parado</span>
          <span class="suave mov-parado">NetGuard desligado até {{ m.ate ? hora(m.ate) : '?' }}: ninguém foi observado nesse tempo</span>
        </template>
        <template v-else>
          <span class="mov-tipo" :class="m.tipo">{{ m.tipo === 'entrou' ? '● entrou' : '○ saiu' }}</span>
          <button type="button" class="mov-nome" @click="abrirPorId(m.dispositivo_id)">{{ m.nome }}</button>
          <span class="mono suave">{{ m.ip }}</span>
        </template>
      </li>
    </ul>
    <template #rodape>
      Para descobrir quem é quem: desligue o Wi-Fi do aparelho, religue, clique em “Procurar agora” e veja
      quem entrou. “Saiu” só aparece depois de {{ mov.dados.value?.janela_min ?? 10 }} min sem responder
      (celular com a tela apagada some e volta).
    </template>
  </CartaoIndicador>

  <div v-if="aberto" class="painel-detalhe" role="dialog" aria-modal="true" :aria-label="aberto.nome" @keydown.esc="aberto = null">
    <div class="cortina" @click="aberto = null"></div>
    <section class="detalhe">
      <header>
        <h2>{{ aberto.nome }}</h2>
        <button class="fechar" type="button" aria-label="Fechar" @click="aberto = null">×</button>
      </header>
      <dl class="dados">
        <div><dt>Situação</dt><dd>{{ { agora: 'Na rede agora', sumiu: `Não respondeu à última procura (visto ${haQuantoTempo(aberto.ultimo_visto)})`, fora: `Fora da rede (visto ${haQuantoTempo(aberto.ultimo_visto)})` }[situacao(aberto)] }}</dd></div>
        <div><dt>IP</dt><dd class="mono">{{ aberto.ip }}</dd></div>
        <div><dt>MAC</dt><dd class="mono">{{ aberto.mac }}</dd></div>
        <div><dt>Fabricante</dt><dd>{{ aberto.mac_aleatorio ? 'oculto (MAC aleatório)' : aberto.fabricante || '—' }}</dd></div>
        <div><dt>Nome na rede</dt><dd>{{ aberto.nome_descoberto || '—' }}</dd></div>
        <div><dt>Primeira vez</dt><dd>{{ dataHora(aberto.primeiro_visto) }}</dd></div>
      </dl>

      <template v-if="!aberto.este_aparelho">
        <label class="campo">
          Apelido
          <input v-model="apelido" maxlength="60" placeholder="ex.: Celular da Ana" />
        </label>
        <label class="campo">
          Notas
          <textarea v-model="notas" maxlength="500" rows="2"></textarea>
        </label>
        <div class="acoes">
          <button class="botao" type="button" :disabled="salvando" @click="alterar(aberto.id, { confiavel: !aberto.confiavel })">
            {{ aberto.confiavel ? 'Tirar de conhecidos' : 'Marcar como conhecido' }}
          </button>
          <button class="botao botao-primario" type="button" :disabled="salvando" @click="salvarDetalhe">Salvar</button>
        </div>
      </template>
      <p v-if="erroDetalhe" class="erro" role="alert">{{ erroDetalhe }}</p>

      <div v-if="resumo7d && !aberto.este_aparelho" class="resumo7d">
        <h3>Últimos 7 dias</h3>
        <p v-if="resumo7d.entradas === 0 && resumo7d.segundos === 0" class="suave">Não esteve na rede nos últimos 7 dias.</p>
        <dl v-else class="numeros">
          <div><dt>Entrou</dt><dd>{{ resumo7d.entradas }} {{ resumo7d.entradas === 1 ? 'vez' : 'vezes' }}</dd></div>
          <div><dt>Tempo na rede</dt><dd>{{ duracao(resumo7d.segundos) }}</dd></div>
          <div v-if="resumo7d.entradas > 0"><dt>Em média por vez</dt><dd>{{ duracao(resumo7d.media_s) }}</dd></div>
          <div v-if="resumo7d.hora_chegada !== null"><dt>Costuma chegar</dt><dd>por volta das {{ resumo7d.hora_chegada }}h</dd></div>
        </dl>
      </div>

      <h3>Quando esteve na rede</h3>
      <Carregando v-if="!aparicoes && !erroDetalhe" />
      <ul v-else-if="aparicoes" class="aparicoes">
        <li v-for="a in aparicoes" :key="a.inicio">
          {{ dataHora(a.inicio) }} → {{ dataHora(a.fim) }}
          <span class="suave">({{ duracao((new Date(a.fim).getTime() - new Date(a.inicio).getTime()) / 1000) }})</span>
        </li>
      </ul>
    </section>
  </div>

  <Confirmar
    :aberto="confirmarVarredura !== null"
    :titulo="confirmarVarredura === 'ligar' ? 'Ligar a procura automática?' : 'Procurar aparelhos agora?'"
    :mensagem="confirmarVarredura === 'ligar'
      ? 'O NetGuard vai mandar um ping para cada endereço da rede de casa (10.0.0.x) a cada 5 minutos, para os aparelhos se apresentarem. Não lê o que eles trafegam. Dá para desligar nas Configurações.'
      : 'Vai mandar um ping para cada endereço da rede de casa, uma vez.'"
    :texto-confirmar="confirmarVarredura === 'ligar' ? 'Ligar' : 'Procurar'"
    @confirmar="confirmarAcao"
    @cancelar="confirmarVarredura = null"
  />
</template>

<style scoped>
.cabecalho {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 0.75rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}
h1 {
  font-size: 1.5rem;
}
.bloco {
  margin-bottom: 1rem;
}
.aviso {
  margin-top: 0;
  color: var(--primaria);
  font-weight: 500;
}
.procura {
  display: flex;
  align-items: center;
  gap: 1rem;
  background: var(--superficie);
  border: 1px solid var(--primaria);
  border-radius: var(--raio);
  padding: 0.85rem 1rem;
}
.procura-texto {
  flex: 1;
  display: flex;
  flex-direction: column;
  gap: 0.35rem;
  min-width: 0;
  font-size: 0.9rem;
}
.barra-procura {
  height: 0.5rem;
  border-radius: 999px;
  background: var(--superficie-2);
  overflow: hidden;
}
.barra-procura span {
  display: block;
  height: 100%;
  background: var(--primaria);
  transition: width 0.6s ease;
}
.radar {
  position: relative;
  width: 2.6rem;
  height: 2.6rem;
  flex: none;
  border-radius: 50%;
  border: 2px solid var(--primaria);
  overflow: hidden;
}
.radar span {
  position: absolute;
  inset: 0;
  background: conic-gradient(from 0deg, color-mix(in srgb, var(--primaria) 55%, transparent), transparent 35%);
  animation: girar-radar 1.6s linear infinite;
}
@keyframes girar-radar {
  to {
    transform: rotate(360deg);
  }
}
.concluida {
  margin-top: 0;
  color: var(--ok);
  font-weight: 500;
}
.linha.chegou {
  animation: chegar 1.6s ease;
  background: color-mix(in srgb, var(--ok) 10%, transparent);
}
@keyframes chegar {
  from {
    background: color-mix(in srgb, var(--ok) 35%, transparent);
  }
}
.acabou {
  font-size: 0.72rem;
  font-weight: 700;
  color: var(--ok);
  border: 1px solid var(--ok);
  border-radius: 999px;
  padding: 0 0.45rem;
}
@media (prefers-reduced-motion: reduce) {
  .radar span,
  .linha.chegou {
    animation: none;
  }
}
.topo-mov {
  margin-top: 1rem;
}
.movimentos {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
  font-size: 0.88rem;
}
.movimentos li {
  display: grid;
  grid-template-columns: 3.2rem 5rem minmax(0, 1fr) auto;
  gap: 0.5rem;
  align-items: center;
  padding: 0.2rem 0;
  border-bottom: 1px solid var(--borda);
}
.movimentos li:last-child {
  border-bottom: 0;
}
.mov-hora {
  font-variant-numeric: tabular-nums;
  color: var(--texto-suave);
}
.mov-tipo.entrou {
  color: var(--ok);
  font-weight: 600;
}
.mov-tipo.saiu {
  color: var(--texto-suave);
}
.mov-tipo.desligado {
  color: var(--alerta);
  font-weight: 600;
}
.mov-parado {
  grid-column: 3 / -1;
}
.mov-nome {
  background: none;
  border: 0;
  padding: 0;
  text-align: left;
  color: var(--texto);
  overflow-wrap: anywhere;
  text-decoration: underline dotted;
}
.faixa {
  display: flex;
  gap: 1rem;
  align-items: center;
  justify-content: space-between;
  flex-wrap: wrap;
  background: var(--superficie);
  border: 1px dashed var(--borda);
  border-radius: var(--raio);
  padding: 0.85rem 1rem;
}
.faixa > div {
  flex: 1 1 20rem;
}
.novos {
  background: var(--alerta-fundo);
  color: var(--texto);
  border-left: 4px solid var(--alerta);
  border-radius: var(--raio);
  padding: 0.85rem 1rem;
}
.novos ul {
  list-style: none;
  margin: 0.5rem 0 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
}
.novos li {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 0.5rem;
  flex-wrap: wrap;
}
.acoes-novo {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}
.resumo {
  display: flex;
  gap: 0.5rem 1.5rem;
  flex-wrap: wrap;
  align-items: baseline;
}
.resumo strong {
  font-size: 1.5rem;
  font-variant-numeric: tabular-nums;
}
.filtros {
  display: flex;
  gap: 2px;
  background: var(--superficie-2);
  border: 1px solid var(--borda);
  border-radius: 8px;
  padding: 2px;
}
.filtro {
  border: 0;
  background: none;
  color: var(--texto-suave);
  border-radius: 6px;
  padding: 0.2rem 0.6rem;
  font-size: 0.85rem;
}
.filtro.ativo {
  background: var(--superficie);
  color: var(--texto);
  font-weight: 600;
}
.busca,
.campo input,
.campo textarea {
  font: inherit;
  width: 100%;
  padding: 0.5rem 0.65rem;
  border: 1px solid var(--borda);
  border-radius: 8px;
  background: var(--superficie-2);
  color: var(--texto);
}
.busca {
  margin-bottom: 0.5rem;
}
.vazio {
  margin: 0.5rem 0;
}
.lista {
  list-style: none;
  margin: 0;
  padding: 0;
}
.linha {
  display: flex;
  align-items: center;
  gap: 0.75rem;
  width: 100%;
  text-align: left;
  background: none;
  border: 0;
  border-bottom: 1px solid var(--borda);
  padding: 0.6rem 0.25rem;
  color: inherit;
}
.linha:hover {
  background: var(--superficie-2);
}
.linha.fora .principal,
.linha.fora .tecnico {
  opacity: 0.6;
}
.lista li:last-child .linha {
  border-bottom: 0;
}
.ponto {
  width: 0.6rem;
  height: 0.6rem;
  border-radius: 50%;
  flex: none;
}
.ponto.agora {
  background: var(--ok);
}
.ponto.sumiu {
  background: linear-gradient(90deg, var(--alerta) 50%, transparent 50%);
  border: 2px solid var(--alerta);
}
.ponto.fora {
  background: transparent;
  border: 2px solid var(--neutro);
}
.sumiu-texto {
  color: var(--alerta);
  white-space: nowrap;
}
.principal {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-width: 0;
}
.nome {
  font-weight: 500;
  display: flex;
  gap: 0.4rem;
  align-items: center;
  flex-wrap: wrap;
  overflow-wrap: anywhere;
}
.sub {
  font-size: 0.82rem;
}
.tecnico {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  font-size: 0.85rem;
  font-variant-numeric: tabular-nums;
  flex: none;
}
.painel-detalhe {
  position: fixed;
  inset: 0;
  z-index: 30;
  display: flex;
  justify-content: flex-end;
}
.cortina {
  position: absolute;
  inset: 0;
  background: rgb(0 0 0 / 40%);
}
.detalhe {
  position: relative;
  width: min(26rem, 100vw);
  height: 100%;
  overflow-y: auto;
  background: var(--superficie);
  border-left: 1px solid var(--borda);
  padding: 1rem 1.2rem;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.detalhe header {
  display: flex;
  justify-content: space-between;
  gap: 0.5rem;
  align-items: flex-start;
}
.detalhe h2 {
  font-size: 1.15rem;
  overflow-wrap: anywhere;
}
.detalhe h3 {
  font-size: 0.95rem;
  margin-top: 0.5rem;
}
.fechar {
  background: none;
  border: 0;
  font-size: 1.5rem;
  line-height: 1;
  color: var(--texto-suave);
}
.dados {
  margin: 0;
  display: grid;
  gap: 0.35rem;
}
.dados div {
  display: flex;
  justify-content: space-between;
  gap: 1rem;
  font-size: 0.9rem;
}
dt {
  color: var(--texto-suave);
  white-space: nowrap;
}
dd {
  margin: 0;
  text-align: right;
  overflow-wrap: anywhere;
}
.campo {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  font-size: 0.9rem;
  font-weight: 500;
}
.acoes {
  display: flex;
  gap: 0.5rem;
  justify-content: flex-end;
  flex-wrap: wrap;
}
.erro {
  color: var(--erro);
  margin: 0;
}
.resumo7d h3 {
  margin-bottom: 0.4rem;
}
.numeros {
  margin: 0;
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 0.5rem;
}
.numeros div {
  background: var(--superficie-2);
  border-radius: 8px;
  padding: 0.45rem 0.6rem;
}
.numeros dt {
  font-size: 0.75rem;
  color: var(--texto-suave);
}
.numeros dd {
  margin: 0;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  text-align: left;
}
.aparicoes {
  margin: 0;
  padding-left: 1.1rem;
  font-size: 0.88rem;
  font-variant-numeric: tabular-nums;
  display: flex;
  flex-direction: column;
  gap: 0.25rem;
}
</style>
