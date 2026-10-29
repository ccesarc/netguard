<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { ErroApi, pedir } from '../api/cliente'
import type { Velocidade } from '../api/tipos'
import { usePeriodico } from '../composables/usePeriodico'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { dataHora, haQuantoTempo, mbps, megabytes, milissegundos } from '../formatar'
import CartaoIndicador from './CartaoIndicador.vue'
import GraficoSerie from './GraficoSerie.vue'
import Confirmar from './Confirmar.vue'
import Indisponivel from './Indisponivel.vue'
import Carregando from './Carregando.vue'
import SeloEstado from './SeloEstado.vue'

const { dados, atualizar } = usePeriodico(() => pedir<Velocidade>('GET', '/velocidade?periodo=30d'), 60_000, useSessaoExpirada())

const confirmar = ref(false)
const aviso = ref<string | null>(null)
let acompanhar: ReturnType<typeof setInterval> | undefined

const ultimo = computed(() => dados.value?.estado.ultimo ?? null)
const rodando = computed(() => !!dados.value?.estado.rodando)
const prog = computed(() => dados.value?.estado.progresso ?? null)
const fases = [
  { id: 'resposta', texto: 'Resposta' },
  { id: 'download', texto: 'Download' },
  { id: 'upload', texto: 'Upload' },
] as const
const ordem = { resposta: 0, download: 1, upload: 2 } as const
const feita = (f: keyof typeof ordem) => !!prog.value && ordem[prog.value.fase] > ordem[f]
const comDado = computed(() => (dados.value?.testes ?? []).filter((t) => t.download_mbps !== null || t.upload_mbps !== null))
const serie = computed(() => {
  const ts = comDado.value
  return {
    t: ts.map((x) => Math.floor(new Date(x.em).getTime() / 1000)),
    series: [
      { nome: 'Download', valores: ts.map((x) => x.download_mbps), cor: 1 as const },
      { nome: 'Upload', valores: ts.map((x) => x.upload_mbps), cor: 2 as const },
    ],
  }
})

async function testar() {
  confirmar.value = false
  aviso.value = null
  try {
    await pedir('POST', '/velocidade/testar')
    await atualizar()
    // acompanha até terminar (o teste leva de 10 a 40 s)
    clearInterval(acompanhar)
    acompanhar = setInterval(async () => {
      await atualizar()
      if (!dados.value?.estado.rodando) clearInterval(acompanhar)
    }, 1000)
  } catch (e) {
    aviso.value = e instanceof ErroApi ? e.message : 'Não deu para começar o teste.'
  }
}

onBeforeUnmount(() => clearInterval(acompanhar))
</script>

<template>
  <CartaoIndicador titulo="Velocidade da internet">
    <template #selo>
      <button class="botao" type="button" :disabled="!dados || rodando" @click="confirmar = true">
        {{ rodando ? 'Testando…' : 'Testar agora' }}
      </button>
    </template>

    <Carregando v-if="!dados" />
    <template v-else>
      <p v-if="aviso" class="aviso" role="alert">{{ aviso }}</p>
      <div v-if="rodando" class="ao-vivo" aria-live="polite">
        <div class="fases">
          <span v-for="f in fases" :key="f.id" class="fase" :class="{ atual: prog?.fase === f.id, feita: feita(f.id) }">
            {{ feita(f.id) ? '✔' : '' }} {{ f.texto }}
          </span>
        </div>
        <template v-if="prog && prog.fase !== 'resposta'">
          <div class="barra-teste"><span :style="{ width: `${Math.min(100, (prog.bytes / Math.max(1, prog.total)) * 100)}%` }"></span></div>
          <p class="numero-vivo">{{ prog.mbps_agora ? mbps(prog.mbps_agora) : '…' }}
            <span class="suave">{{ prog.fase === 'download' ? 'baixando' : 'enviando' }} · {{ Math.round(prog.bytes / 1048576) }} de {{ Math.round(prog.total / 1048576) }} MB</span>
          </p>
        </template>
        <p v-else class="suave">Medindo o tempo de resposta…</p>
      </div>

      <div v-if="ultimo" class="ultimo">
        <div>
          <span class="rotulo">Download</span>
          <strong class="numero">{{ ultimo.download_mbps !== null ? mbps(ultimo.download_mbps) : '—' }}</strong>
        </div>
        <div>
          <span class="rotulo">Upload</span>
          <strong class="numero">{{ ultimo.upload_mbps !== null ? mbps(ultimo.upload_mbps) : '—' }}</strong>
        </div>
        <div>
          <span class="rotulo">Resposta</span>
          <strong class="numero menor">{{ ultimo.latencia_ms !== null ? milissegundos(ultimo.latencia_ms) : '—' }}</strong>
        </div>
      </div>
      <p v-if="ultimo" class="suave linha">
        {{ haQuantoTempo(ultimo.em) }} · {{ ultimo.origem === 'manual' ? 'pedido na tela' : 'agendado' }}
        <template v-if="ultimo.servidor"> · servidor {{ ultimo.servidor }}</template>
        <template v-if="ultimo.interface"> · por {{ ultimo.interface }}</template>
      </p>
      <p v-if="ultimo?.erro" class="erro-linha">
        <SeloEstado tom="alerta" texto="Incompleto" /> {{ ultimo.erro }}
      </p>
      <Indisponivel v-if="!ultimo && !rodando" motivo="Nenhum teste ainda. Use “Testar agora” ou espere o próximo agendado." />

      <GraficoSerie
        v-if="comDado.length >= 2"
        class="grafico"
        :t="serie.t"
        :series="serie.series"
        unidade="Mbps"
        :altura="200"
        rotulo="Download e upload de cada teste de velocidade nos últimos 30 dias"
      />
    </template>

    <template v-if="dados" #rodape>
      <template v-if="dados.config.ativo">
        {{ dados.config.vezes_por_dia }}× por dia<template v-if="dados.estado.proximo">, próximo {{ dataHora(dados.estado.proximo) }}</template>
        · cada teste usa ~{{ dados.config.mb_download + dados.config.mb_upload }} MB
        · gasto nos últimos 30 dias: {{ megabytes(dados.gasto_30d_mb) }} (estimativa {{ megabytes(dados.estimativa_mes_mb) }}/mês).
      </template>
      <template v-else>Teste agendado desligado (Configurações). O botão continua funcionando.</template>
      Mede até este aparelho: pelo Wi-Fi, o número inclui o Wi-Fi.
    </template>
  </CartaoIndicador>

  <Confirmar
    :aberto="confirmar"
    titulo="Testar a velocidade agora?"
    :mensagem="`Vai baixar ${dados?.config.mb_download ?? 0} MB e enviar ${dados?.config.mb_upload ?? 0} MB. Durante o teste (até 40 s) a internet da casa fica mais lenta.`"
    texto-confirmar="Testar"
    @confirmar="testar"
    @cancelar="confirmar = false"
  />
</template>

<style scoped>
.ultimo {
  display: flex;
  flex-wrap: wrap;
  gap: 0.75rem 2rem;
}
.ultimo div {
  display: flex;
  flex-direction: column;
}
.rotulo {
  font-size: 0.8rem;
  color: var(--texto-suave);
}
.numero {
  font-size: 1.6rem;
  font-variant-numeric: tabular-nums;
  line-height: 1.2;
}
.numero.menor {
  font-size: 1.2rem;
}
.linha {
  margin: 0.4rem 0 0;
  font-size: 0.85rem;
}
.erro-linha {
  margin: 0.4rem 0 0;
  font-size: 0.85rem;
  display: flex;
  gap: 0.5rem;
  align-items: center;
  flex-wrap: wrap;
}
.aviso {
  margin: 0 0 0.5rem;
  color: var(--alerta);
  font-weight: 500;
}
.grafico {
  margin-top: 0.75rem;
}
.ao-vivo {
  display: flex;
  flex-direction: column;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
}
.fases {
  display: flex;
  gap: 0.4rem;
  flex-wrap: wrap;
}
.fase {
  font-size: 0.78rem;
  padding: 0.1rem 0.6rem;
  border-radius: 999px;
  border: 1px solid var(--borda);
  color: var(--texto-suave);
}
.fase.atual {
  border-color: var(--primaria);
  color: var(--primaria);
  font-weight: 700;
}
.fase.feita {
  color: var(--ok);
  border-color: var(--ok);
}
.barra-teste {
  height: 0.5rem;
  border-radius: 999px;
  background: var(--superficie-2);
  overflow: hidden;
}
.barra-teste span {
  display: block;
  height: 100%;
  background: var(--serie-1);
  transition: width 0.8s ease;
}
.numero-vivo {
  margin: 0;
  font-size: 1.5rem;
  font-weight: 700;
  font-variant-numeric: tabular-nums;
}
.numero-vivo .suave {
  font-size: 0.85rem;
  font-weight: 400;
}
</style>
