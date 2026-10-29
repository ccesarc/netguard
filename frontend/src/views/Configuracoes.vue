<script setup lang="ts">
import { computed, ref } from 'vue'
import { ErroApi, ErroNaoAutenticado, pedir } from '../api/cliente'
import type { Ajustes, Configuracao } from '../api/tipos'
import { useSessaoExpirada } from '../composables/useSessaoExpirada'
import { megabytes } from '../formatar'
import CartaoIndicador from '../components/CartaoIndicador.vue'
import Confirmar from '../components/Confirmar.vue'
import Carregando from '../components/Carregando.vue'
import ErroCarga from '../components/ErroCarga.vue'

// O formulário trabalha com texto (uma linha por item) e % em vez de fração.
interface Formulario {
  intervalo_s: number
  externos: string
  dns_comparacao: string
  urls: string
  limite_latencia_ms: number
  limite_perda_pct: number
  vel_ativo: boolean
  vel_vezes: number
  vel_down: number
  vel_up: number
  varredura: boolean
  plano_down: number
  plano_up: number
}

const expirou = useSessaoExpirada()
const config = ref<Configuracao | null>(null)
const form = ref<Formulario | null>(null)
const erroCarga = ref<string | null>(null)
const errosSalvar = ref<string[]>([])
const salvo = ref(false)
const salvando = ref(false)
const confirmar = ref(false)

function paraForm(a: Ajustes): Formulario {
  return {
    intervalo_s: a.intervalo_s,
    externos: a.externos.join('\n'),
    dns_comparacao: a.dns_comparacao,
    urls: a.urls.join('\n'),
    limite_latencia_ms: a.limite_latencia_ms,
    limite_perda_pct: Math.round(a.limite_perda * 100),
    vel_ativo: a.velocidade.ativo,
    vel_vezes: a.velocidade.vezes_por_dia,
    vel_down: a.velocidade.mb_download,
    vel_up: a.velocidade.mb_upload,
    varredura: a.varredura_ativa,
    plano_down: a.plano_download_mbps,
    plano_up: a.plano_upload_mbps,
  }
}

const linhas = (t: string) =>
  t
    .split('\n')
    .map((l) => l.trim())
    .filter(Boolean)

function deForm(f: Formulario): Ajustes {
  return {
    intervalo_s: Number(f.intervalo_s),
    externos: linhas(f.externos),
    dns_comparacao: f.dns_comparacao.trim(),
    urls: linhas(f.urls),
    limite_latencia_ms: Number(f.limite_latencia_ms),
    limite_perda: Number(f.limite_perda_pct) / 100,
    velocidade: {
      ativo: f.vel_ativo,
      vezes_por_dia: Number(f.vel_vezes),
      mb_download: Number(f.vel_down),
      mb_upload: Number(f.vel_up),
    },
    varredura_ativa: f.varredura,
    plano_download_mbps: Number(f.plano_down) || 0,
    plano_upload_mbps: Number(f.plano_up) || 0,
  }
}

async function carregar() {
  erroCarga.value = null
  try {
    config.value = await pedir<Configuracao>('GET', '/configuracao')
    form.value = paraForm(config.value.ajustes)
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    erroCarga.value = e instanceof Error ? e.message : String(e)
  }
}
carregar()

const nomes: Record<keyof Ajustes, string> = {
  intervalo_s: 'Intervalo entre verificações',
  externos: 'Endereços de fora',
  dns_comparacao: 'DNS de comparação',
  urls: 'Sites testados',
  limite_latencia_ms: 'Limite de latência',
  limite_perda: 'Limite de perda',
  velocidade: 'Teste de velocidade',
  varredura_ativa: 'Procura de aparelhos',
  plano_download_mbps: 'Plano contratado (download)',
  plano_upload_mbps: 'Plano contratado (upload)',
}

const mudancas = computed(() => {
  if (!config.value || !form.value) return []
  const antes = config.value.ajustes
  const depois = deForm(form.value)
  return (Object.keys(nomes) as (keyof Ajustes)[]).filter((k) => JSON.stringify(antes[k]) !== JSON.stringify(depois[k])).map((k) => nomes[k])
})

const consumo = computed(() => {
  const f = form.value
  if (!f || !f.vel_ativo) return 0
  return Number(f.vel_vezes) * (Number(f.vel_down) + Number(f.vel_up)) * 30
})

async function salvar() {
  confirmar.value = false
  if (!form.value) return
  salvando.value = true
  errosSalvar.value = []
  salvo.value = false
  try {
    config.value = await pedir<Configuracao>('PUT', '/configuracao', deForm(form.value))
    form.value = paraForm(config.value.ajustes)
    salvo.value = true
  } catch (e) {
    if (e instanceof ErroNaoAutenticado) return expirou()
    errosSalvar.value = e instanceof ErroApi && e.detalhes.length ? e.detalhes : [e instanceof Error ? e.message : String(e)]
  } finally {
    salvando.value = false
  }
}

function desfazer() {
  if (config.value) form.value = paraForm(config.value.ajustes)
  errosSalvar.value = []
}

function restaurarPadrao() {
  if (config.value) form.value = paraForm(config.value.padrao)
  salvo.value = false
}
</script>

<template>
  <h1 class="titulo">Configurações</h1>
  <ErroCarga v-if="erroCarga" :mensagem="erroCarga" @tentar="carregar" />
  <Carregando v-else-if="!form" />

  <form v-else class="formulario" @submit.prevent="mudancas.length && (confirmar = true)" @input="salvo = false">
    <CartaoIndicador titulo="Verificações">
      <label class="campo">
        <span>Intervalo entre verificações <span class="suave">(segundos, de 5 a 300)</span></span>
        <input v-model.number="form.intervalo_s" type="number" min="5" max="300" required />
      </label>
      <label class="campo">
        <span>Endereços de fora para o ping <span class="suave">(um por linha, até 5)</span></span>
        <textarea v-model="form.externos" rows="3" spellcheck="false" class="mono"></textarea>
      </label>
      <label class="campo">
        <span>DNS de comparação <span class="suave">(se só o DNS configurado falhar, a culpa é dele)</span></span>
        <input v-model="form.dns_comparacao" spellcheck="false" class="mono" required />
      </label>
      <label class="campo">
        <span>Sites testados por HTTPS <span class="suave">(um por linha, até 5; pode deixar vazio)</span></span>
        <textarea v-model="form.urls" rows="3" spellcheck="false" class="mono"></textarea>
      </label>
    </CartaoIndicador>

    <CartaoIndicador titulo="Quando a internet conta como “com problema”">
      <div class="duas">
        <label class="campo">
          <span>Latência acima de <span class="suave">(ms)</span></span>
          <input v-model.number="form.limite_latencia_ms" type="number" min="20" max="2000" required />
        </label>
        <label class="campo">
          <span>Perda de pacotes acima de <span class="suave">(%)</span></span>
          <input v-model.number="form.limite_perda_pct" type="number" min="1" max="50" required />
        </label>
      </div>
      <template #rodape>“Com problema” não é queda: queda é quando nada de fora responde por ~30 s.</template>
    </CartaoIndicador>

    <CartaoIndicador titulo="Teste de velocidade">
      <label class="marcar">
        <input v-model="form.vel_ativo" type="checkbox" />
        Testar sozinho todos os dias
      </label>
      <div class="tres">
        <label class="campo">
          <span>Vezes por dia</span>
          <select v-model.number="form.vel_vezes" :disabled="!form.vel_ativo">
            <option v-for="n in [1, 2, 3, 4, 6, 8, 12]" :key="n" :value="n">{{ n }}</option>
          </select>
        </label>
        <label class="campo">
          <span>Download <span class="suave">(MB)</span></span>
          <input v-model.number="form.vel_down" type="number" min="5" max="200" required />
        </label>
        <label class="campo">
          <span>Upload <span class="suave">(MB)</span></span>
          <input v-model.number="form.vel_up" type="number" min="2" max="100" required />
        </label>
      </div>
      <template #rodape>
        Gasto estimado: <strong>{{ megabytes(consumo) }} por mês</strong>. Os testes ficam espalhados no dia a
        partir das 7h. Testes maiores medem melhor conexões rápidas.
      </template>
    </CartaoIndicador>

    <CartaoIndicador titulo="Plano de internet contratado">
      <div class="duas">
        <label class="campo">
          <span>Download <span class="suave">(Mbps, no cabo)</span></span>
          <input v-model.number="form.plano_down" type="number" min="0" max="10000" />
        </label>
        <label class="campo">
          <span>Upload <span class="suave">(Mbps, no cabo)</span></span>
          <input v-model.number="form.plano_up" type="number" min="0" max="10000" />
        </label>
      </div>
      <template #rodape>Usado na tela Saúde para comparar com os testes de velocidade. 0 = não sei.</template>
    </CartaoIndicador>

    <CartaoIndicador titulo="Procura de aparelhos">
      <label class="marcar">
        <input v-model="form.varredura" type="checkbox" />
        Procurar aparelhos na rede a cada 5 minutos
      </label>
      <template #rodape>
        Manda um ping para cada endereço da rede de casa para os aparelhos se apresentarem. Não lê o que
        eles trafegam. Desligada, a lista de Dispositivos só mostra quem conversou com este PC.
      </template>
    </CartaoIndicador>

    <div v-if="errosSalvar.length" class="erros" role="alert">
      <strong>Não salvou:</strong>
      <ul>
        <li v-for="e in errosSalvar" :key="e">{{ e }}</li>
      </ul>
    </div>
    <p v-if="salvo" class="salvo" role="status">Salvo. Já está valendo, sem reiniciar.</p>

    <div class="acoes">
      <button class="botao" type="button" @click="restaurarPadrao">Valores de fábrica</button>
      <button class="botao" type="button" :disabled="!mudancas.length" @click="desfazer">Desfazer</button>
      <button class="botao botao-primario" type="submit" :disabled="!mudancas.length || salvando">
        {{ salvando ? 'Salvando…' : 'Salvar' }}
      </button>
    </div>
  </form>

  <Confirmar
    :aberto="confirmar"
    titulo="Salvar as configurações?"
    :mensagem="`Vai mudar: ${mudancas.join(', ')}. Passa a valer na hora.`"
    texto-confirmar="Salvar"
    @confirmar="salvar"
    @cancelar="confirmar = false"
  />
</template>

<style scoped>
.titulo {
  font-size: 1.5rem;
  margin-bottom: 1rem;
}
.formulario {
  display: flex;
  flex-direction: column;
  gap: 1rem;
  max-width: 48rem;
}
.campo {
  display: flex;
  flex-direction: column;
  gap: 0.3rem;
  font-size: 0.9rem;
  font-weight: 500;
  margin-bottom: 0.75rem;
  min-width: 0;
}
.campo:last-child {
  margin-bottom: 0;
}
.campo .suave {
  font-weight: 400;
}
input,
textarea,
select {
  font: inherit;
  padding: 0.5rem 0.65rem;
  border: 1px solid var(--borda);
  border-radius: 8px;
  background: var(--superficie-2);
  color: var(--texto);
  width: 100%;
}
textarea {
  resize: vertical;
}
.marcar {
  display: flex;
  align-items: center;
  gap: 0.5rem;
  margin-bottom: 0.75rem;
  font-weight: 500;
}
.marcar input {
  width: auto;
}
.duas,
.tres {
  display: grid;
  gap: 0 1rem;
}
.duas {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 14rem), 1fr));
}
.tres {
  grid-template-columns: repeat(auto-fit, minmax(min(100%, 9rem), 1fr));
}
.erros {
  background: var(--erro-fundo);
  color: var(--erro);
  border-radius: var(--raio);
  padding: 0.75rem 1rem;
}
.erros ul {
  margin: 0.3rem 0 0;
  padding-left: 1.2rem;
}
.salvo {
  margin: 0;
  color: var(--ok);
  font-weight: 500;
}
.acoes {
  display: flex;
  justify-content: flex-end;
  gap: 0.5rem;
  flex-wrap: wrap;
}
</style>
