<script setup lang="ts">
import { computed } from 'vue'
import { pedir } from '../api/cliente'
import type { Disponibilidades, Historico, RelatorioSaude, VisaoGeral } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { dataHora, duracao, haQuantoTempo, mbps, milissegundos, porcentagem, qualidadeSinal } from '../formatar'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import CartaoDisponibilidade from '../components/CartaoDisponibilidade.vue'
import GraficoSerie from '../components/GraficoSerie.vue'
import SeloEstado, { type Tom } from '../components/SeloEstado.vue'
import Indisponivel from '../components/Indisponivel.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

const expirou = useSessaoExpirada()

const { dados, erro, carregando, atualizar } = usePeriodico(
  () => pedir<VisaoGeral>('GET', '/visao-geral'),
  10_000,
  expirou,
)
const { dados: hist } = usePeriodico(() => pedir<Historico>('GET', '/internet/historico?periodo=1h'), 60_000, expirou)
const { dados: disp } = usePeriodico(() => pedir<Disponibilidades>('GET', '/internet/disponibilidade'), 60_000, expirou)
const { dados: saude } = usePeriodico(() => pedir<RelatorioSaude>('GET', '/saude-da-rede'), 120_000, expirou)

const nomesTipo = { wifi: 'Wi-Fi', cabo: 'Cabo', outro: 'Outro' } as const

const seloInterface = computed<{ tom: Tom; texto: string }>(() => {
  const i = dados.value?.interface
  if (!i?.disponivel) return { tom: 'neutro', texto: 'Sem dados' }
  return i.ok ? { tom: 'ok', texto: 'Conectado' } : { tom: 'erro', texto: 'Com problema' }
})

const seloGateway = computed<{ tom: Tom; texto: string }>(() => {
  const g = dados.value?.gateway
  if (!g?.disponivel) return { tom: 'neutro', texto: 'Sem dados' }
  if (!g.ok) return { tom: 'erro', texto: 'Não responde' }
  if (g.perda > 0) return { tom: 'alerta', texto: 'Perdendo pacotes' }
  return { tom: 'ok', texto: 'Respondendo' }
})

const seloInternet = computed<{ tom: Tom; texto: string }>(() => {
  const n = dados.value?.internet
  if (!n?.disponivel) return { tom: 'neutro', texto: 'Sem dados' }
  if (n.estado === 'fora') return { tom: 'erro', texto: 'Fora' }
  if (n.estado === 'degradado') return { tom: 'alerta', texto: 'Com problema' }
  return { tom: 'ok', texto: 'No ar' }
})

const seloDNS = computed<{ tom: Tom; texto: string }>(() => {
  const d = dados.value?.dns
  if (!d?.disponivel) return { tom: 'neutro', texto: 'Sem dados' }
  return d.ok ? { tom: 'ok', texto: 'Respondendo' } : { tom: 'erro', texto: 'Não responde' }
})

const seriesHora = computed(() =>
  hist.value
    ? [
        { nome: 'Roteador', valores: hist.value.gateway_ms, cor: 1 as const },
        { nome: 'Internet', valores: hist.value.internet_ms, cor: 2 as const },
        { nome: 'DNS', valores: hist.value.dns_ms, cor: 3 as const },
      ]
    : [],
)
const temPontos = computed(() => !!hist.value && hist.value.gateway_ms.some((v) => v !== null))

const futuros = computed(() => {
  const d = dados.value
  if (!d) return []
  return [
    { titulo: 'Anúncios bloqueados', motivo: d.bloqueio.motivo },
    { titulo: 'Alertas recentes', motivo: d.alertas.motivo },
  ]
})
</script>

<template>
  <div class="cabecalho">
    <h1>Visão geral</h1>
    <p v-if="dados?.medido_em" class="suave atualizado">
      Medido {{ haQuantoTempo(dados.medido_em) }} · atualiza a cada 10 s
    </p>
  </div>

  <ErroCarga v-if="erro" :mensagem="erro" class="bloco" @tentar="atualizar" />
  <Carregando v-if="carregando && !dados" />

  <template v-if="dados">
    <div v-if="dados.queda" class="queda bloco" role="alert">
      <strong>{{ dados.queda.causa.texto }}</strong>
      <span>desde {{ dataHora(dados.queda.inicio) }} · há {{ duracao(dados.queda.duracao_s) }}</span>
    </div>

    <RouterLink v-if="saude" to="/saude" class="saude-faixa">
      <span class="saude-nota">{{ saude.nota ?? '—' }}</span>
      <span>
        <strong>Saúde: {{ saude.rotulo }}</strong>
        <span v-if="saude.recomendacoes?.length" class="suave"> · próxima melhoria: {{ saude.recomendacoes[0].titulo.toLowerCase() }}</span>
      </span>
      <span class="link-mais">ver tudo →</span>
    </RouterLink>

    <div class="grade">
      <CartaoIndicador titulo="Internet">
        <template #selo><SeloEstado v-bind="seloInternet" /></template>
        <Indisponivel v-if="!dados.internet.disponivel" :motivo="dados.internet.motivo" />
        <template v-else>
          <p v-if="dados.internet.latencia_media_ms !== null" class="numero">
            {{ milissegundos(dados.internet.latencia_media_ms) }}
            <span class="suave unidade">latência média até fora de casa</span>
          </p>
          <p class="causa" :class="dados.internet.estado">{{ dados.internet.causa.texto }}</p>
          <p v-if="dados.internet.detalhe" class="suave detalhe">{{ dados.internet.detalhe }}</p>
          <dl class="dados">
            <div v-if="dados.internet.perda !== null">
              <dt>Perda</dt>
              <dd>{{ porcentagem(dados.internet.perda) }}</dd>
            </div>
          </dl>
        </template>
      </CartaoIndicador>

      <CartaoIndicador titulo="Roteador (gateway)">
        <template #selo><SeloEstado v-bind="seloGateway" /></template>
        <Indisponivel v-if="!dados.gateway.disponivel" :motivo="dados.gateway.motivo" />
        <template v-else>
          <p class="numero" v-if="dados.gateway.ok">
            {{ milissegundos(dados.gateway.latencia_media_ms) }}
            <span class="suave unidade">latência média</span>
          </p>
          <p class="problema" v-else>{{ dados.gateway.erro || 'Sem resposta' }}</p>
          <dl class="dados">
            <div>
              <dt>Endereço</dt>
              <dd class="mono">{{ dados.gateway.alvo }}</dd>
            </div>
            <div>
              <dt>Perda</dt>
              <dd>
                {{ porcentagem(dados.gateway.perda) }}
                <span class="suave">({{ dados.gateway.recebidos }}/{{ dados.gateway.enviados }})</span>
              </dd>
            </div>
          </dl>
        </template>
      </CartaoIndicador>

      <CartaoIndicador titulo="DNS">
        <template #selo><SeloEstado v-bind="seloDNS" /></template>
        <Indisponivel v-if="!dados.dns.disponivel" :motivo="dados.dns.motivo" />
        <dl v-else class="dados">
          <div v-for="m in dados.dns.sistema" :key="'s' + m.alvo">
            <dt>Configurado <span class="mono">{{ m.alvo }}</span></dt>
            <dd :class="{ problema: !m.ok }">{{ m.ok ? milissegundos(m.ms) : m.erro || 'falhou' }}</dd>
          </div>
          <div v-for="m in dados.dns.externo" :key="'e' + m.alvo">
            <dt>Comparação <span class="mono">{{ m.alvo }}</span></dt>
            <dd :class="{ problema: !m.ok }">{{ m.ok ? milissegundos(m.ms) : m.erro || 'falhou' }}</dd>
          </div>
        </dl>
      </CartaoIndicador>

      <CartaoIndicador titulo="Este aparelho">
        <template #selo><SeloEstado v-bind="seloInterface" /></template>
        <Indisponivel v-if="!dados.interface.disponivel" :motivo="dados.interface.motivo ?? ''" />
        <template v-else>
          <dl class="dados">
            <div v-if="dados.wifi.disponivel">
              <dt>Wi-Fi</dt>
              <dd>
                {{ dados.wifi.amostra.ssid }}
                <span class="suave">· sinal {{ dados.wifi.amostra.sinal }}% ({{ qualidadeSinal(dados.wifi.amostra.sinal).texto.toLowerCase() }})</span>
              </dd>
            </div>
            <div>
              <dt>Interface</dt>
              <dd>
                <span class="mono">{{ dados.interface.nome }}</span>
                <span v-if="dados.interface.tipo" class="suave"> · {{ nomesTipo[dados.interface.tipo] }}</span>
              </dd>
            </div>
            <div>
              <dt>Endereço</dt>
              <dd class="mono">{{ dados.interface.ipv4 || '—' }}</dd>
            </div>
          </dl>
          <p v-if="dados.interface.problema" class="problema">{{ dados.interface.problema }}</p>
        </template>
      </CartaoIndicador>
    </div>

    <div class="linha-grafico">
      <CartaoIndicador titulo="Última hora — latência" class="cartao-grafico">
        <template #selo>
          <RouterLink to="/internet" class="link-mais">ver mais períodos →</RouterLink>
        </template>
        <GraficoSerie
          v-if="hist && temPontos"
          :t="hist.t"
          :series="seriesHora"
          :fora="hist.fora"
          :passo-s="hist.passo_s"
          unidade="ms"
          rotulo="Latência do roteador, da internet e do DNS na última hora"
        />
        <Indisponivel v-else-if="hist" motivo="Ainda sem medições na última hora." />
        <Carregando v-else />
      </CartaoIndicador>

      <div class="coluna">
        <CartaoIndicador titulo="Disponibilidade">
          <CartaoDisponibilidade v-if="disp" titulo="Últimas 24 h" :d="disp['24h']" />
          <Carregando v-else />
          <template #rodape>Conta só o tempo em que o monitor estava ligado.</template>
        </CartaoIndicador>
        <CartaoIndicador titulo="Velocidade">
          <template v-if="dados.velocidade.disponivel">
            <p class="velocidade">
              <span><span class="suave">↓</span> {{ dados.velocidade.ultimo.download_mbps !== null ? mbps(dados.velocidade.ultimo.download_mbps) : '—' }}</span>
              <span><span class="suave">↑</span> {{ dados.velocidade.ultimo.upload_mbps !== null ? mbps(dados.velocidade.ultimo.upload_mbps) : '—' }}</span>
            </p>
            <p class="suave detalhe">
              {{ dados.velocidade.rodando ? 'Testando agora…' : `Último teste ${haQuantoTempo(dados.velocidade.ultimo.em)}` }}
            </p>
          </template>
          <Indisponivel v-else :motivo="dados.velocidade.motivo" />
          <template #rodape><RouterLink to="/internet" class="link-mais">testar e ver o histórico →</RouterLink></template>
        </CartaoIndicador>
      </div>
    </div>

    <div class="grade">
      <CartaoIndicador titulo="Dispositivos">
        <template v-if="dados.dispositivos.disponivel" #selo>
          <SeloEstado v-if="dados.dispositivos.resumo.novos" tom="alerta" :texto="`${dados.dispositivos.resumo.novos} novo${dados.dispositivos.resumo.novos > 1 ? 's' : ''}`" />
        </template>
        <template v-if="dados.dispositivos.disponivel">
          <p class="numero">
            {{ dados.dispositivos.resumo.na_rede }}
            <span class="suave unidade">na rede agora · {{ dados.dispositivos.resumo.total }} já vistos</span>
          </p>
          <p v-if="!dados.dispositivos.varredura_ativa" class="suave detalhe">Procura automática desligada: a lista fica incompleta.</p>
        </template>
        <Indisponivel v-else :motivo="dados.dispositivos.motivo" />
        <template #rodape><RouterLink to="/dispositivos" class="link-mais">ver aparelhos →</RouterLink></template>
      </CartaoIndicador>
      <CartaoIndicador v-for="f in futuros" :key="f.titulo" :titulo="f.titulo">
        <template #selo><SeloEstado tom="neutro" texto="Em breve" /></template>
        <Indisponivel :motivo="f.motivo" />
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
.queda {
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 0.75rem;
  align-items: baseline;
  background: var(--erro-fundo);
  color: var(--erro);
  border-radius: var(--raio);
  padding: 0.75rem 1rem;
}
.grade {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(min(100%, 16rem), 1fr));
  gap: 1rem;
  margin-bottom: 1rem;
}
.linha-grafico {
  display: grid;
  grid-template-columns: minmax(0, 3fr) minmax(0, 1fr);
  gap: 1rem;
  margin-bottom: 1rem;
}
@media (max-width: 1000px) {
  .linha-grafico {
    grid-template-columns: minmax(0, 1fr);
  }
}
.saude-faixa {
  display: flex;
  align-items: center;
  gap: 0.9rem;
  flex-wrap: wrap;
  background: var(--superficie);
  border: 1px solid var(--borda);
  border-radius: var(--raio);
  padding: 0.65rem 1rem;
  margin-bottom: 1rem;
  color: var(--texto);
  text-decoration: none;
}
.saude-faixa:hover {
  border-color: var(--texto-suave);
}
.saude-nota {
  font-size: 1.5rem;
  font-weight: 800;
  font-variant-numeric: tabular-nums;
}
.saude-faixa .link-mais {
  margin-left: auto;
}
.coluna {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  min-width: 0;
}
.velocidade {
  margin: 0;
  display: flex;
  flex-wrap: wrap;
  gap: 0.25rem 1.25rem;
  font-size: 1.35rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.link-mais {
  font-size: 0.85rem;
  color: var(--primaria);
  text-decoration: none;
}
.numero {
  margin: 0;
  font-size: 1.9rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
  line-height: 1.1;
}
.unidade {
  display: block;
  font-size: 0.8rem;
  font-weight: 400;
}
.causa {
  margin: 0.5rem 0 0;
  font-weight: 500;
}
.causa.fora {
  color: var(--erro);
}
.causa.degradado {
  color: var(--alerta);
}
.detalhe {
  margin: 0.15rem 0 0;
  font-size: 0.85rem;
}
.dados {
  margin: 0.6rem 0 0;
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
  font-variant-numeric: tabular-nums;
  overflow-wrap: anywhere;
  min-width: 0;
}
.problema {
  margin: 0.5rem 0 0;
  color: var(--erro);
  font-weight: 500;
}
dd.problema {
  margin: 0;
}
</style>
