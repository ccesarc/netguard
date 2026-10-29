<script setup lang="ts">
import { computed } from 'vue'
import { pedir } from '../api/cliente'
import type { Computador, EstadoSaude, ItemSaude, RelatorioSaude } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { haQuantoTempo } from '../formatar'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

const expirou = useSessaoExpirada()
const { dados, erro, atualizar } = usePeriodico(() => pedir<RelatorioSaude>('GET', '/saude-da-rede'), 60_000, expirou)
const pc = usePeriodico(() => pedir<Computador>('GET', '/computador'), 30_000, expirou)

// Ícone + texto: o estado nunca depende só da cor.
const marca: Record<EstadoSaude, { icone: string; texto: string }> = {
  ok: { icone: '✔', texto: 'Bom' },
  dica: { icone: '➜', texto: 'Dica' },
  atencao: { icone: '!', texto: 'Atenção' },
  problema: { icone: '✖', texto: 'Problema' },
  sem_dados: { icone: '…', texto: 'Sem dados' },
}

const ordemAreas = ['Internet', 'Wi-Fi da casa', 'Aparelhos', 'Este aparelho', 'Computador']
const areas = computed(() => {
  const grupos = new Map<string, ItemSaude[]>()
  for (const it of dados.value?.itens ?? []) {
    if (!grupos.has(it.area)) grupos.set(it.area, [])
    grupos.get(it.area)!.push(it)
  }
  return [...grupos.entries()].sort((a, b) => ordemAreas.indexOf(a[0]) - ordemAreas.indexOf(b[0]))
})

const tomNota = computed(() => {
  const n = dados.value?.nota
  if (n == null) return 'neutro'
  return n >= 75 ? 'ok' : n >= 55 ? 'alerta' : 'erro'
})

const leitura = computed(() => (pc.dados.value?.disponivel ? pc.dados.value.resumo : null))
const pct = (usado: number, total: number) => Math.round((usado / Math.max(1, total)) * 100)
const gb = (mb: number) => (mb / 1024).toLocaleString('pt-BR', { maximumFractionDigits: 1 })
</script>

<template>
  <div class="cabecalho">
    <h1>Saúde</h1>
    <p v-if="dados" class="suave atualizado">Avaliado {{ haQuantoTempo(dados.gerado_em) }} · atualiza a cada minuto</p>
  </div>

  <ErroCarga v-if="erro" :mensagem="erro" class="bloco" @tentar="atualizar" />
  <Carregando v-if="!dados && !erro" />

  <template v-if="dados">
    <section class="nota bloco" :class="tomNota" aria-label="Nota geral">
      <div class="numero-nota">{{ dados.nota ?? '—' }}<span v-if="dados.nota !== null" class="de">/100</span></div>
      <div>
        <strong class="rotulo">{{ dados.rotulo }}</strong>
        <p class="suave">
          {{ dados.itens.length - dados.sem_dados }} de {{ dados.itens.length }} pontos avaliados com dados reais.
          <template v-if="dados.sem_dados">Os outros esperam mais medições.</template>
        </p>
      </div>
    </section>

    <CartaoIndicador v-if="dados.recomendacoes?.length" titulo="O que fazer, do mais ao menos importante" class="bloco">
      <ol class="fazer">
        <li v-for="r in dados.recomendacoes" :key="r.id" :class="r.estado">
          <span class="marca" :class="r.estado"><span aria-hidden="true">{{ marca[r.estado].icone }}</span> {{ marca[r.estado].texto }}</span>
          <strong>{{ r.titulo }}:</strong> {{ r.recomendacao }}
          <RouterLink v-if="r.link" :to="r.link" class="ver">ver →</RouterLink>
        </li>
      </ol>
    </CartaoIndicador>

    <div class="areas">
      <CartaoIndicador v-for="[area, itens] in areas" :key="area" :titulo="area">
        <ul class="itens">
          <li v-for="it in itens" :key="it.id">
            <span class="marca" :class="it.estado" :title="marca[it.estado].texto">
              <span aria-hidden="true">{{ marca[it.estado].icone }}</span>
              <span class="so-leitor">{{ marca[it.estado].texto }}</span>
            </span>
            <div>
              <strong>{{ it.titulo }}</strong>
              <p class="resumo">{{ it.resumo }}</p>
              <p v-if="it.prova" class="prova suave">{{ it.prova }}</p>
            </div>
          </li>
        </ul>

        <div v-if="area === 'Computador' && leitura" class="medidores">
          <div>
            <span class="suave">Memória livre</span>
            <span class="barra"><span :style="{ width: `${pct(leitura.agora.mem_disponivel_mb, leitura.agora.mem_total_mb)}%` }"></span></span>
            <span>{{ gb(leitura.agora.mem_disponivel_mb) }} de {{ gb(leitura.agora.mem_total_mb) }} GB</span>
          </div>
          <div v-for="d in leitura.agora.discos ?? []" :key="d.ponto">
            <span class="suave">Disco {{ d.ponto }}</span>
            <span class="barra uso"><span :style="{ width: `${d.uso_pct}%` }"></span></span>
            <span>{{ Math.round(d.uso_pct) }}% usado · {{ Math.round(d.livre_gb) }} GB livres</span>
          </div>
          <div v-if="leitura.agora.temp_cpu !== null">
            <span class="suave">Temperatura da CPU</span>
            <span class="barra quente"><span :style="{ width: `${Math.min(100, leitura.agora.temp_cpu)}%` }"></span></span>
            <span>{{ Math.round(leitura.agora.temp_cpu) }} °C<template v-if="leitura.temp_max_hora !== null"> (máx. {{ Math.round(leitura.temp_max_hora) }} °C na hora)</template></span>
          </div>
          <div>
            <span class="suave">Processador</span>
            <span class="barra uso"><span :style="{ width: `${Math.min(100, (leitura.agora.carga_5 / leitura.agora.nucleos) * 100)}%` }"></span></span>
            <span>{{ Math.round((leitura.agora.carga_5 / leitura.agora.nucleos) * 100) }}% de {{ leitura.agora.nucleos }} núcleos</span>
          </div>
        </div>
        <template v-if="area === 'Computador'" #rodape>
          Lido a cada minuto de arquivos que o sistema já mantém: não pesa no computador. A saúde interna do
          SSD (desgaste) precisaria de permissão de administrador e fica de fora.
        </template>
      </CartaoIndicador>
    </div>
  </template>
</template>

<style scoped>
.cabecalho {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 0.5rem 1rem;
  flex-wrap: wrap;
  margin-bottom: 1rem;
}
h1 {
  font-size: 1.5rem;
}
.atualizado {
  margin: 0;
  font-size: 0.85rem;
}
.bloco {
  margin-bottom: 1rem;
}
.nota {
  display: flex;
  align-items: center;
  gap: 1.25rem;
  background: var(--superficie);
  border: 1px solid var(--borda);
  border-left: 6px solid var(--neutro);
  border-radius: var(--raio);
  padding: 1rem 1.25rem;
}
.nota.ok {
  border-left-color: var(--ok);
}
.nota.alerta {
  border-left-color: var(--alerta);
}
.nota.erro {
  border-left-color: var(--erro);
}
.numero-nota {
  font-size: 2.6rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
  line-height: 1;
}
.de {
  font-size: 1rem;
  font-weight: 500;
  color: var(--texto-suave);
}
.rotulo {
  font-size: 1.3rem;
}
.nota p {
  margin: 0.2rem 0 0;
  font-size: 0.88rem;
}
.fazer {
  margin: 0;
  padding-left: 1.3rem;
  display: flex;
  flex-direction: column;
  gap: 0.6rem;
  font-size: 0.92rem;
}
.ver {
  color: var(--primaria);
  text-decoration: none;
  white-space: nowrap;
}
.marca {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 0.25rem;
  font-size: 0.75rem;
  font-weight: 700;
  border-radius: 999px;
  padding: 0.05rem 0.5rem;
  margin-right: 0.35rem;
  white-space: nowrap;
}
.marca.ok {
  color: var(--ok);
  background: var(--ok-fundo);
}
.marca.dica {
  color: var(--primaria);
  background: color-mix(in srgb, var(--primaria) 12%, transparent);
}
.marca.atencao {
  color: var(--alerta);
  background: var(--alerta-fundo);
}
.marca.problema {
  color: var(--erro);
  background: var(--erro-fundo);
}
.marca.sem_dados {
  color: var(--neutro);
  background: var(--neutro-fundo);
}
.areas {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 24rem), 1fr));
  gap: 1rem;
  align-items: start;
}
.itens {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 0.75rem;
}
.itens li {
  display: grid;
  grid-template-columns: 1.9rem minmax(0, 1fr);
  gap: 0.4rem;
  align-items: start;
}
.itens .marca {
  width: 1.7rem;
  height: 1.7rem;
  padding: 0;
  margin: 0;
  font-size: 0.85rem;
}
.resumo {
  margin: 0.1rem 0 0;
  font-size: 0.9rem;
}
.prova {
  margin: 0.1rem 0 0;
  font-size: 0.8rem;
  overflow-wrap: anywhere;
}
.so-leitor {
  position: absolute;
  width: 1px;
  height: 1px;
  overflow: hidden;
  clip: rect(0 0 0 0);
}
.medidores {
  margin-top: 1rem;
  padding-top: 0.75rem;
  border-top: 1px solid var(--borda);
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  font-size: 0.85rem;
}
.medidores div {
  display: grid;
  grid-template-columns: 9rem minmax(4rem, 1fr) auto;
  align-items: center;
  gap: 0.6rem;
}
@media (max-width: 480px) {
  .medidores div {
    grid-template-columns: 1fr;
    gap: 0.2rem;
  }
}
.barra {
  height: 0.5rem;
  background: var(--superficie-2);
  border: 1px solid var(--borda);
  border-radius: 999px;
  overflow: hidden;
}
.barra span {
  display: block;
  height: 100%;
  background: var(--ok);
}
.barra.uso span {
  background: var(--serie-1);
}
.barra.quente span {
  background: var(--alerta);
}
</style>
