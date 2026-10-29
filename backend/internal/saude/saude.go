// Package saude junta o que o NetGuard mediu e escreve, em português, como
// está a rede e o que dá para melhorar. Função pura: recebe os números, devolve
// o relatório. Cada item traz a prova (o número que levou à conclusão) e, quando
// falta dado, diz isso em vez de chutar.
package saude

import (
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"netguard/internal/armazenamento"
	"netguard/internal/computador"
	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

type Estado string

const (
	OK       Estado = "ok"
	Dica     Estado = "dica"     // não é problema, mas dá para melhorar
	Atencao  Estado = "atencao"  // vale olhar
	Problema Estado = "problema" // está atrapalhando
	SemDados Estado = "sem_dados"
)

type Item struct {
	ID           string `json:"id"`
	Area         string `json:"area"`
	Titulo       string `json:"titulo"`
	Estado       Estado `json:"estado"`
	Resumo       string `json:"resumo"`
	Prova        string `json:"prova,omitempty"`
	Recomendacao string `json:"recomendacao,omitempty"`
	Link         string `json:"link,omitempty"` // tela onde ver o detalhe
	peso         float64
}

type Relatorio struct {
	GeradoEm      time.Time `json:"gerado_em"`
	Nota          *int      `json:"nota"` // nil enquanto houver pouco dado
	Rotulo        string    `json:"rotulo"`
	Itens         []Item    `json:"itens"`
	Recomendacoes []Item    `json:"recomendacoes"` // o que fazer, do mais ao menos importante
	SemDados      int       `json:"sem_dados"`
}

type Dispositivos struct {
	Total, NaRede, Novos int
	ProcuraAtiva         bool
}

type Entrada struct {
	Agora         time.Time
	InterfaceTipo string // wifi, cabo

	Cobertura7dS float64 // segundos com o monitor rodando nos últimos 7 dias
	Quedas7d     []incidentes.Incidente

	Resumo24h armazenamento.ResumoRede
	Resumo7d  armazenamento.ResumoRede
	PorHora7d []armazenamento.PontoHistorico // pontos de 1 h dos últimos 7 dias

	Testes7d           []velocidade.Resultado
	PlanoDown, PlanoUp int

	WifiAgora *wifi.Amostra
	Wifi24h   []armazenamento.PontoWifi
	Vizinhos  *wifi.Vizinhanca

	Dispositivos Dispositivos
	DNSSistema   string
	DNSExterno   string

	Computador *computador.Resumo
}

func Avaliar(e Entrada) Relatorio {
	itens := []Item{
		estabilidade(e),
		quedasWifiPC(e),
		latencia(e),
		perda(e),
		pico(e),
		velocidadePlano(e),
		filaCheia(e),
		dns(e),
		sinalWifi(e),
		canal(e),
		congestionamento(e),
		aparelhos(e),
		seguranca(e),
		memoria(e),
		processador(e),
		disco(e),
		temperatura(e),
		ligado(e),
	}
	var r Relatorio
	r.GeradoEm = e.Agora
	for _, it := range itens {
		if it.ID == "" {
			continue // não se aplica (ex.: Wi-Fi no cabo)
		}
		it.Resumo, it.Prova, it.Recomendacao = virgula(it.Resumo), virgula(it.Prova), virgula(it.Recomendacao)
		r.Itens = append(r.Itens, it)
		if it.Estado == SemDados {
			r.SemDados++
		}
		if it.Recomendacao != "" && it.Estado != OK {
			r.Recomendacoes = append(r.Recomendacoes, it)
		}
	}
	gravidade := map[Estado]int{Problema: 0, Atencao: 1, Dica: 2, SemDados: 3}
	sort.SliceStable(r.Recomendacoes, func(i, j int) bool {
		a, b := r.Recomendacoes[i], r.Recomendacoes[j]
		if gravidade[a.Estado] != gravidade[b.Estado] {
			return gravidade[a.Estado] < gravidade[b.Estado]
		}
		return a.peso > b.peso
	})
	r.Nota, r.Rotulo = nota(r.Itens)
	return r
}

// nota: começa em 100 e desconta por problema e atenção, pesando o que mais
// importa no dia a dia. Sem nota enquanto metade ou mais estiver sem dados.
func nota(itens []Item) (*int, string) {
	var avaliados, semDados int
	for _, it := range itens {
		if (it.ID == "estabilidade" || it.ID == "latencia") && it.Estado == SemDados {
			return nil, "Ainda juntando dados" // sem o básico da internet, nota enganaria
		}
	}
	n := 100.0
	for _, it := range itens {
		switch it.Estado {
		case SemDados:
			semDados++
			continue
		case Problema:
			n -= 15 * it.peso
		case Atencao:
			n -= 6 * it.peso
		}
		avaliados++
	}
	if avaliados == 0 || semDados*2 >= len(itens) {
		return nil, "Ainda juntando dados"
	}
	v := int(math.Round(max(0, n)))
	switch {
	case v >= 90:
		return &v, "Ótima"
	case v >= 75:
		return &v, "Boa"
	case v >= 55:
		return &v, "Regular"
	}
	return &v, "Ruim"
}

// ---- verificações ----

func estabilidade(e Entrada) Item {
	it := Item{ID: "estabilidade", Area: "Internet", Titulo: "Estabilidade", Link: "/eventos", peso: 2}
	if e.Cobertura7dS < 6*3600 {
		it.Estado, it.Resumo = SemDados, "Menos de 6 horas de medição nos últimos 7 dias."
		return it
	}
	var n int
	var fora time.Duration
	causas := map[diagnostico.Causa]int{}
	for _, q := range e.Quedas7d {
		if q.Causa == diagnostico.CausaRedeLocal {
			continue // é a placa deste PC, não a internet: item próprio
		}
		n++
		fim := e.Agora
		if q.Fim != nil {
			fim = *q.Fim
		}
		fora += fim.Sub(q.Inicio)
		causas[q.Causa]++
	}
	cob := fmt.Sprintf("medido em %.0f%% dos últimos 7 dias", min(100, e.Cobertura7dS/(7*86400)*100))
	if n == 0 {
		it.Estado, it.Resumo, it.Prova = OK, "Nenhuma queda da internet em 7 dias.", cob
		return it
	}
	it.Resumo = fmt.Sprintf("%d queda%s em 7 dias, %s fora no total.", n, s(n), duracao(fora))
	principal, vezes := diagnostico.CausaExterno, 0
	for c, v := range causas {
		if v > vezes {
			principal, vezes = c, v
		}
	}
	it.Prova = fmt.Sprintf("causa mais comum: %s (%d de %d); %s", strings.ToLower(diagnostico.Texto(principal)), vezes, n, cob)
	switch principal {
	case diagnostico.CausaGateway:
		it.Recomendacao = "As quedas são do roteador (ele parou de responder). Confira a tomada e a temperatura dele; se continuar, peça troca ao provedor."
	case diagnostico.CausaDNSConfigurado, diagnostico.CausaDNS:
		it.Recomendacao = "As quedas são de DNS. Trocar o DNS do PC (ou do roteador) para o de comparação costuma resolver."
	default:
		it.Recomendacao = "As quedas vêm de fora de casa. Anote os horários (tela Histórico de eventos) e mostre ao provedor."
	}
	if n >= 3 || fora >= 30*time.Minute {
		it.Estado = Problema
	} else {
		it.Estado = Atencao
	}
	return it
}

func quedasWifiPC(e Entrada) Item {
	if e.InterfaceTipo != "wifi" {
		return Item{}
	}
	it := Item{ID: "wifi_pc", Area: "Este aparelho", Titulo: "Wi-Fi deste PC", Link: "/eventos", peso: 0.5}
	if e.Cobertura7dS < 6*3600 {
		it.Estado, it.Resumo = SemDados, "Menos de 6 horas de medição nos últimos 7 dias."
		return it
	}
	n := 0
	for _, q := range e.Quedas7d {
		if q.Causa == diagnostico.CausaRedeLocal {
			n++
		}
	}
	if n == 0 {
		it.Estado, it.Resumo = OK, "A placa Wi-Fi deste PC não caiu em 7 dias."
		return it
	}
	it.Estado = Atencao
	it.Resumo = fmt.Sprintf("A placa Wi-Fi deste PC caiu %d vez%s em 7 dias (não foi a internet da casa).", n, map[bool]string{true: "", false: "es"}[n == 1])
	it.Recomendacao = "Ligar este PC no cabo acaba com isso e deixa as medições mais fiéis. No Raspberry Pi, use o cabo."
	return it
}

func latencia(e Entrada) Item {
	it := Item{ID: "latencia", Area: "Internet", Titulo: "Tempo de resposta", Link: "/internet", peso: 1}
	r := e.Resumo24h
	if r.Amostras < 360 || r.LatenciaMediana == nil {
		it.Estado, it.Resumo = SemDados, "Menos de 1 hora de medição nas últimas 24 h."
		return it
	}
	m := *r.LatenciaMediana
	it.Prova = fmt.Sprintf("mediana de %.0f ms nas últimas 24 h até servidores fora de casa", m)
	switch {
	case m < 60:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Resposta rápida: %.0f ms.", m)
	case m < 120:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("Resposta um pouco lenta: %.0f ms.", m)
		it.Recomendacao = "Bom para navegar e ver vídeo; pode incomodar em jogos e chamadas. Se piorou de repente, fale com o provedor."
	default:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Resposta lenta: %.0f ms.", m)
		it.Recomendacao = "Acima de 120 ms já atrapalha chamadas e jogos. Mostre o gráfico da tela Internet ao provedor."
	}
	return it
}

func perda(e Entrada) Item {
	it := Item{ID: "perda", Area: "Internet", Titulo: "Perda de pacotes", Link: "/internet", peso: 1}
	r := e.Resumo24h
	if r.Amostras < 360 || r.PerdaMedia == nil {
		it.Estado, it.Resumo = SemDados, "Menos de 1 hora de medição nas últimas 24 h."
		return it
	}
	p := *r.PerdaMedia * 100
	it.Prova = fmt.Sprintf("%.2f%% dos pacotes se perderam nas últimas 24 h", p)
	switch {
	case p < 0.5:
		it.Estado, it.Resumo = OK, "Quase nada se perde no caminho."
	case p < 2:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("Um pouco de perda: %.1f%%.", p)
		it.Recomendacao = "Perda baixa, mas existe: chamadas podem picotar. Se for sempre, fale com o provedor."
	default:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Perda alta: %.1f%%.", p)
		it.Recomendacao = "Perda acima de 2% atrapalha tudo. Reinicie o roteador; se continuar, chame o provedor."
	}
	return it
}

// pico procura as horas do dia em que a internet piora, com pelo menos 2 dias de dado.
func pico(e Entrada) Item {
	it := Item{ID: "pico", Area: "Internet", Titulo: "Horário de pico", Link: "/internet", peso: 1}
	somas := map[int][]float64{}
	horas := 0
	for _, p := range e.PorHora7d {
		if p.InternetMS == nil {
			continue
		}
		t := p.Em.In(time.Local)
		somas[t.Hour()] = append(somas[t.Hour()], *p.InternetMS)
		horas++
	}
	if horas < 36 || len(somas) < 18 { // ~1,5 dia medido, cobrindo quase todas as horas
		it.Estado, it.Resumo = SemDados, "Precisa de 1 a 2 dias de medição para comparar os horários do dia."
		return it
	}
	medias := map[int]float64{}
	var todas []float64
	for h, v := range somas {
		medias[h] = media(v)
		todas = append(todas, medias[h])
	}
	slices.Sort(todas)
	base := todas[len(todas)/2]
	pior := -1
	for h, m := range medias {
		if pior < 0 || m > medias[pior] {
			pior = h
		}
	}
	if medias[pior] < 1.8*base || medias[pior]-base < 20 {
		it.Estado, it.Resumo = OK, "A internet fica igual o dia todo."
		it.Prova = fmt.Sprintf("hora mais lenta (%dh) com %.0f ms; normal %.0f ms", pior, medias[pior], base)
		return it
	}
	// faixa de horas seguidas em volta da pior que também estão lentas
	de, ate := pior, pior
	for medias[(de+23)%24] >= 1.5*base && (de+23)%24 != pior {
		de = (de + 23) % 24
	}
	for medias[(ate+1)%24] >= 1.5*base && (ate+1)%24 != de {
		ate = (ate + 1) % 24
	}
	it.Estado = Atencao
	it.Resumo = fmt.Sprintf("Entre %dh e %dh a internet fica mais lenta.", de, (ate+1)%24)
	it.Prova = fmt.Sprintf("às %dh a resposta média é %.0f ms; no resto do dia, %.0f ms (%.1f×)", pior, medias[pior], base, medias[pior]/base)
	it.Recomendacao = "É o horário de pico do provedor (todo mundo usando). Se atrapalhar chamadas ou jogos nesse horário, mostre o gráfico ao provedor."
	return it
}

func velocidadePlano(e Entrada) Item {
	it := Item{ID: "velocidade", Area: "Internet", Titulo: "Velocidade × plano", Link: "/internet", peso: 1.5}
	var down, up []float64
	wifiN := 0
	for _, t := range e.Testes7d {
		if t.DownloadMbps != nil {
			down = append(down, *t.DownloadMbps)
		}
		if t.UploadMbps != nil {
			up = append(up, *t.UploadMbps)
		}
		if strings.HasPrefix(t.Interface, "wl") {
			wifiN++
		}
	}
	if len(down) == 0 {
		it.Estado, it.Resumo = SemDados, "Nenhum teste de velocidade nos últimos 7 dias."
		it.Recomendacao = "Rode um teste na tela Internet (“Testar agora”) ou espere o próximo agendado."
		return it
	}
	md, mu := mediana(down), mediana(up)
	pelaWifi := wifiN*2 >= len(e.Testes7d)
	meio := "no cabo"
	esperado := 0.85
	if pelaWifi {
		meio, esperado = "pelo Wi-Fi", 0.6
	}
	if e.PlanoDown == 0 {
		it.Estado = SemDados
		it.Resumo = fmt.Sprintf("Mediana %.0f ↓ / %.0f ↑ Mbps %s em %d teste%s; falta o plano para comparar.", md, mu, meio, len(down), s(len(down)))
		it.Recomendacao = "Informe o plano contratado em Configurações para comparar."
		return it
	}
	rd := md / float64(e.PlanoDown)
	ru := 1.0
	if e.PlanoUp > 0 && len(up) > 0 {
		ru = mu / float64(e.PlanoUp)
	}
	pior := min(rd, ru)
	it.Prova = fmt.Sprintf("mediana %.0f ↓ / %.0f ↑ Mbps em %d teste%s %s = %.0f%% / %.0f%% do plano %d/%d (%s, o normal é %.0f%% ou mais)",
		md, mu, len(down), s(len(down)), meio, rd*100, ru*100, e.PlanoDown, e.PlanoUp, meio, esperado*100)
	switch {
	case pior >= esperado:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Entregando o esperado: %.0f%% do download e %.0f%% do upload do plano.", rd*100, ru*100)
	case pior >= esperado*0.7:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("Um pouco abaixo do esperado: %.0f%% do download e %.0f%% do upload do plano.", rd*100, ru*100)
	default:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Bem abaixo do plano: %.0f%% do download e %.0f%% do upload.", rd*100, ru*100)
	}
	if it.Estado != OK {
		if pelaWifi {
			it.Recomendacao = "Teste pelo cabo (ou com o PC perto do roteador): se no cabo chegar perto do plano, o limite é o Wi-Fi; se não, é com o provedor."
		} else {
			it.Recomendacao = "No cabo devia chegar perto do plano. Repita em horários diferentes e, se continuar, mostre os testes ao provedor."
		}
	}
	return it
}

// filaCheia: quanto a resposta piora enquanto a internet está ocupada (bufferbloat).
func filaCheia(e Entrada) Item {
	it := Item{ID: "fila", Area: "Internet", Titulo: "Internet ocupada (fila cheia)", Link: "/internet", peso: 1}
	var parado, baixando, enviando []float64
	for _, t := range e.Testes7d {
		if t.LatenciaMS == nil {
			continue
		}
		if t.LatenciaDownloadMS != nil {
			parado = append(parado, *t.LatenciaMS)
			baixando = append(baixando, *t.LatenciaDownloadMS)
		}
		if t.LatenciaUploadMS != nil {
			enviando = append(enviando, *t.LatenciaUploadMS)
		}
	}
	if len(baixando) == 0 {
		it.Estado, it.Resumo = SemDados, "Medido junto com o teste de velocidade; ainda não houve teste com essa medição."
		return it
	}
	p, b := mediana(parado), mediana(baixando)
	piora := b - p
	if len(enviando) > 0 {
		piora = max(piora, mediana(enviando)-p)
	}
	nota := "F"
	for _, n := range []struct {
		lim  float64
		nota string
	}{{5, "A+"}, {30, "A"}, {60, "B"}, {200, "C"}, {400, "D"}} {
		if piora < n.lim {
			nota = n.nota
			break
		}
	}
	it.Prova = fmt.Sprintf("resposta parada %.0f ms → baixando %.0f ms", p, b)
	if len(enviando) > 0 {
		it.Prova += fmt.Sprintf(" → enviando %.0f ms", mediana(enviando))
	}
	it.Prova += fmt.Sprintf(" (nota %s)", nota)
	switch {
	case piora < 30:
		it.Estado, it.Resumo = OK, fmt.Sprintf("A internet continua respondendo bem mesmo ocupada (nota %s).", nota)
	case piora < 100:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("Quando alguém baixa ou envia muito, a resposta piora %.0f ms (nota %s).", piora, nota)
	default:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Quando alguém baixa ou envia muito, a internet “trava”: +%.0f ms (nota %s).", piora, nota)
	}
	if it.Estado != OK {
		it.Recomendacao = "Se o roteador tiver “QoS”, “SQM” ou “controle de banda inteligente”, ligar resolve: chamadas e jogos param de travar quando alguém baixa ou assiste vídeo."
	}
	return it
}

func dns(e Entrada) Item {
	it := Item{ID: "dns", Area: "Internet", Titulo: "DNS", Link: "/internet", peso: 0.5}
	r := e.Resumo7d
	if r.DNSSistemaMS == nil || r.DNSExternoMS == nil || r.Amostras < 360 {
		it.Estado, it.Resumo = SemDados, "Menos de 1 hora de medição de DNS."
		return it
	}
	sis, ext := *r.DNSSistemaMS, *r.DNSExternoMS
	falhas := 0.0
	if r.DNSSistemaFalhas != nil {
		falhas = *r.DNSSistemaFalhas
	}
	it.Prova = fmt.Sprintf("configurado (%s) %.0f ms, comparação (%s) %.0f ms; falhas do configurado %.1f%%", e.DNSSistema, sis, e.DNSExterno, ext, falhas*100)
	switch {
	case falhas > 0.02:
		it.Estado = Atencao
		it.Resumo = fmt.Sprintf("O DNS configurado falhou em %.1f%% das consultas.", falhas*100)
		it.Recomendacao = fmt.Sprintf("Trocar o DNS para %s (no PC ou no roteador) deixa os sites abrindo sem engasgar.", e.DNSExterno)
	case sis-ext >= 15 && ext < sis*0.8:
		it.Estado = Dica
		it.Resumo = fmt.Sprintf("O DNS %s responde %.0f ms mais rápido que o configurado.", e.DNSExterno, sis-ext)
		it.Recomendacao = fmt.Sprintf("Opcional: usar %s como DNS faz cada site começar a abrir um pouco mais rápido.", e.DNSExterno)
	default:
		it.Estado, it.Resumo = OK, fmt.Sprintf("DNS respondendo bem: %.0f ms.", sis)
	}
	return it
}

func sinalWifi(e Entrada) Item {
	if e.InterfaceTipo != "wifi" {
		return Item{}
	}
	it := Item{ID: "sinal", Area: "Este aparelho", Titulo: "Sinal do Wi-Fi", Link: "/internet", peso: 0.7}
	var soma, n float64
	pior := 101.0
	for _, p := range e.Wifi24h {
		if p.Sinal != nil {
			soma += *p.Sinal
			n++
		}
		if p.SinalMin != nil {
			pior = min(pior, *p.SinalMin)
		}
	}
	if n == 0 {
		if e.WifiAgora == nil {
			it.Estado, it.Resumo = SemDados, "Sem leitura do Wi-Fi ainda."
			return it
		}
		soma, n, pior = float64(e.WifiAgora.Sinal), 1, float64(e.WifiAgora.Sinal)
	}
	m := soma / n
	it.Prova = fmt.Sprintf("média de %.0f%% nas últimas 24 h; o pior momento foi %.0f%%", m, pior)
	switch {
	case m >= 60 && pior >= 30:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Sinal bom: %.0f%%.", m)
	case m >= 40:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("Sinal médio (%.0f%%) ou com quedas (até %.0f%%).", m, pior)
		it.Recomendacao = "Aproximar o PC do roteador, tirar obstáculos (parede, espelho, micro-ondas) ou usar o cabo."
	default:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Sinal fraco: %.0f%%.", m)
		it.Recomendacao = "Sinal fraco derruba a velocidade. Use o cabo ou um repetidor/mesh perto deste PC."
	}
	return it
}

func canal(e Entrada) Item {
	it := Item{ID: "canal", Area: "Wi-Fi da casa", Titulo: "Canal do roteador", Link: "/internet", peso: 0.7}
	if e.Vizinhos == nil {
		it.Estado, it.Resumo = SemDados, "Ainda não houve busca das redes por perto."
		return it
	}
	var trocar []string
	for _, f := range []wifi.Faixa{e.Vizinhos.F5, e.Vizinhos.F24} {
		if f.Mudar {
			trocar = append(trocar, fmt.Sprintf("%s: do %d para o %d", f.Nome, f.CanalAtual, f.Recomendado))
		}
	}
	it.Prova = fmt.Sprintf("5 GHz no canal %d · 2,4 GHz no canal %d · média de %d busca%s", e.Vizinhos.F5.CanalAtual, e.Vizinhos.F24.CanalAtual, e.Vizinhos.Buscas, s(e.Vizinhos.Buscas))
	if len(trocar) == 0 {
		it.Estado, it.Resumo = OK, "Os canais do roteador já são os melhores disponíveis."
		return it
	}
	it.Estado = Atencao
	it.Resumo = "Há canal mais livre: " + strings.Join(trocar, "; ") + "."
	it.Recomendacao = "Trocar o canal no painel do roteador (" + strings.Join(trocar, "; ") + "). Precisa da senha do roteador; todos desconectam por alguns segundos."
	return it
}

func congestionamento(e Entrada) Item {
	if e.Vizinhos == nil {
		return Item{}
	}
	it := Item{ID: "faixa24", Area: "Wi-Fi da casa", Titulo: "Faixa de 2,4 GHz", Link: "/internet", peso: 0.3}
	n := e.Vizinhos.F24.Vizinhos
	it.Prova = fmt.Sprintf("%d redes de vizinhos em 2,4 GHz; %d em 5 GHz", n, e.Vizinhos.F5.Vizinhos)
	if n < 10 {
		it.Estado, it.Resumo = OK, "Pouca gente disputando o 2,4 GHz."
		return it
	}
	it.Estado = Dica
	it.Resumo = fmt.Sprintf("O 2,4 GHz está lotado (%d redes por perto).", n)
	if e.Vizinhos.F5.TuaRede != "" {
		it.Recomendacao = fmt.Sprintf("Perto do roteador, conecte celulares, TV e notebook na rede de 5 GHz (%s): mais rápida e bem mais livre. O 2,4 GHz fica para cômodos longe.", e.Vizinhos.F5.TuaRede)
	} else {
		it.Recomendacao = "Se o roteador tiver 5 GHz, use-o perto dele: é mais rápido e mais livre."
	}
	return it
}

func aparelhos(e Entrada) Item {
	it := Item{ID: "aparelhos", Area: "Aparelhos", Titulo: "Quem está na rede", Link: "/dispositivos", peso: 0.7}
	d := e.Dispositivos
	if d.Total == 0 {
		it.Estado, it.Resumo = SemDados, "Nenhum aparelho observado ainda."
		it.Recomendacao = "Na tela Dispositivos, ligue a procura automática."
		return it
	}
	it.Prova = fmt.Sprintf("%d na rede agora, %d já vistos", d.NaRede, d.Total)
	switch {
	case d.Novos > 0:
		it.Estado = Atencao
		it.Resumo = fmt.Sprintf("%d aparelho%s novo%s sem conferir.", d.Novos, s(d.Novos), s(d.Novos))
		it.Recomendacao = "Confira na tela Dispositivos: se for de casa, marque “É conhecido”; se não reconhecer, troque a senha do Wi-Fi."
	case !d.ProcuraAtiva:
		it.Estado = Dica
		it.Resumo = "Procura automática desligada: a lista pode estar incompleta."
		it.Recomendacao = "Ligue a procura na tela Dispositivos para saber sempre quem está na rede."
	default:
		it.Estado, it.Resumo = OK, "Nenhum aparelho desconhecido."
	}
	return it
}

func seguranca(e Entrada) Item {
	if e.Vizinhos == nil {
		return Item{}
	}
	it := Item{ID: "seguranca", Area: "Wi-Fi da casa", Titulo: "Proteção do Wi-Fi", peso: 1}
	var tuas []string
	pior := ""
	for _, r := range e.Vizinhos.Redes {
		if !r.Propria && !r.Conectada {
			continue
		}
		tuas = append(tuas, fmt.Sprintf("%s: %s", r.SSID, aberta(r.Seguranca)))
		switch {
		case r.Seguranca == "" || strings.Contains(r.Seguranca, "WEP"):
			pior = "aberta"
		case strings.Contains(r.Seguranca, "WPA1") && !strings.Contains(r.Seguranca, "WPA2") && pior != "aberta":
			pior = "wpa1"
		case strings.Contains(r.Seguranca, "WPA3") && pior == "":
			pior = "wpa3"
		case pior == "" || pior == "wpa3":
			pior = "wpa2"
		}
	}
	if len(tuas) == 0 {
		it.Estado, it.Resumo = SemDados, "As redes do teu roteador não apareceram na busca."
		return it
	}
	it.Prova = strings.Join(tuas, " · ")
	switch pior {
	case "aberta":
		it.Estado, it.Resumo = Problema, "Uma rede tua está aberta ou com proteção antiga (WEP)."
		it.Recomendacao = "Coloque WPA2 (ou WPA3) com senha forte no painel do roteador."
	case "wpa1":
		it.Estado, it.Resumo = Problema, "Uma rede tua usa WPA, que já foi quebrado."
		it.Recomendacao = "Mude para WPA2 ou WPA3 no painel do roteador."
	case "wpa2":
		it.Estado, it.Resumo = Dica, "Protegida com WPA2 (bom)."
		it.Recomendacao = "Se o roteador oferecer “WPA3” ou “WPA2/WPA3”, use: é mais resistente a quem tenta adivinhar a senha."
	default:
		it.Estado, it.Resumo = OK, "Protegida com WPA3."
	}
	return it
}

// ---- computador ----

const areaPC = "Computador"

func semLeitura(id, titulo string) Item {
	return Item{ID: id, Area: areaPC, Titulo: titulo, Estado: SemDados, Resumo: "Aguardando a primeira leitura."}
}

func memoria(e Entrada) Item {
	if e.Computador == nil {
		return semLeitura("memoria", "Memória")
	}
	a := e.Computador.Agora
	it := Item{ID: "memoria", Area: areaPC, Titulo: "Memória", peso: 0.7}
	livre := float64(a.MemDisponivelMB) / float64(max(1, a.MemTotalMB))
	pressao := 0.0
	if a.PressaoMem != nil {
		pressao = *a.PressaoMem
	}
	it.Prova = fmt.Sprintf("%.1f de %.1f GB livres (o menor na última hora: %.1f GB) · pressão de memória %.1f%% nos últimos 5 min",
		gb(a.MemDisponivelMB), gb(a.MemTotalMB), gb(e.Computador.MemMinHoraMB), pressao)
	if a.SwapTotalMB > 0 && a.SwapUsadoMB > 0 {
		it.Prova += fmt.Sprintf(" · swap usado %.1f GB", gb(a.SwapUsadoMB))
	}
	switch {
	case livre < 0.05 || pressao >= 10:
		it.Estado, it.Resumo = Problema, "Falta memória: programas estão esperando e o sistema pode fechar algum à força."
	case livre < 0.15 || pressao >= 1:
		it.Estado, it.Resumo = Atencao, "Memória apertada."
	default:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Sobra memória: %.1f GB livres.", gb(a.MemDisponivelMB))
	}
	if it.Estado != OK {
		it.Recomendacao = "Feche o que pesa (navegador com muitas abas, emulador, WordPress parados). No terminal: `painel` → Memória e o que mais pesa."
	}
	return it
}

func processador(e Entrada) Item {
	if e.Computador == nil {
		return semLeitura("cpu", "Processador")
	}
	a := e.Computador.Agora
	it := Item{ID: "cpu", Area: areaPC, Titulo: "Processador", peso: 0.5}
	uso := a.Carga5 / float64(max(1, a.Nucleos))
	pressao := 0.0
	if a.PressaoCPU != nil {
		pressao = *a.PressaoCPU
	}
	it.Prova = fmt.Sprintf("carga %.2f para %d núcleos (%.0f%%) · esperando CPU %.1f%% do tempo nos últimos 5 min", a.Carga5, a.Nucleos, uso*100, pressao)
	switch {
	case uso >= 1.5 || pressao >= 25:
		it.Estado, it.Resumo = Problema, "Processador sobrecarregado."
		it.Recomendacao = "Algo está usando o processador sem parar. No terminal: `painel` → O que está rodando."
	case uso >= 0.7 || pressao >= 10:
		it.Estado, it.Resumo = Atencao, "Processador bem ocupado."
		it.Recomendacao = "Se não tiver nada pesado rodando de propósito, veja o que está usando: `painel` → O que está rodando."
	default:
		it.Estado, it.Resumo = OK, "Processador folgado."
	}
	return it
}

func disco(e Entrada) Item {
	if e.Computador == nil {
		return semLeitura("disco", "Disco")
	}
	a := e.Computador.Agora
	it := Item{ID: "disco", Area: areaPC, Titulo: "Disco", peso: 0.7}
	if len(a.Discos) == 0 {
		it.Estado, it.Resumo = SemDados, "Não deu para ler o espaço em disco."
		return it
	}
	pior := a.Discos[0]
	var partes []string
	for _, d := range a.Discos {
		partes = append(partes, fmt.Sprintf("%s: %.0f%% usado, %.0f GB livres", d.Ponto, d.UsoPct, d.LivreGB))
		if d.UsoPct > pior.UsoPct {
			pior = d
		}
	}
	it.Prova = strings.Join(partes, " · ")
	espera := 0.0
	if a.PressaoDisco != nil {
		espera = *a.PressaoDisco
		it.Prova += fmt.Sprintf(" · esperando o disco %.1f%% do tempo nos últimos 5 min", espera)
	}
	switch {
	case pior.UsoPct >= 90:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("Disco quase cheio: %.0f%% em %s.", pior.UsoPct, pior.Ponto)
		it.Recomendacao = "Libere espaço: imagens antigas do Docker (`docker system prune` com cuidado), downloads, caches. Disco cheio trava o sistema e o banco."
	case pior.UsoPct >= 80 || espera >= 10:
		it.Estado = Atencao
		if pior.UsoPct >= 80 {
			it.Resumo = fmt.Sprintf("Disco enchendo: %.0f%% em %s.", pior.UsoPct, pior.Ponto)
			it.Recomendacao = "Vale liberar espaço antes de chegar a 90%."
		} else {
			it.Resumo = "O disco anda ocupado: programas esperando por ele."
			it.Recomendacao = "Algo está lendo ou gravando muito (backup, Docker, indexação). Se for sempre, veja em `painel` → O que está rodando."
		}
	default:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Espaço de sobra: %.0f GB livres.", pior.LivreGB)
	}
	return it
}

func temperatura(e Entrada) Item {
	if e.Computador == nil {
		return semLeitura("temperatura", "Temperatura")
	}
	it := Item{ID: "temperatura", Area: areaPC, Titulo: "Temperatura", peso: 0.7}
	a := e.Computador.Agora
	if a.TempCPU == nil {
		it.Estado, it.Resumo = SemDados, "Este computador não informa a temperatura da CPU."
		return it
	}
	pico := *a.TempCPU
	if e.Computador.TempMaxHora != nil {
		pico = max(pico, *e.Computador.TempMaxHora)
	}
	it.Prova = fmt.Sprintf("CPU agora %.0f °C; o máximo na última hora foi %.0f °C (sensor %s)", *a.TempCPU, pico, a.TempSensor)
	switch {
	case pico >= 85:
		it.Estado, it.Resumo = Problema, fmt.Sprintf("CPU muito quente: chegou a %.0f °C.", pico)
		it.Recomendacao = "Acima de 85 °C o processador se freia sozinho e a peça sofre. Limpe a poeira, deixe as saídas de ar livres e evite rodar coisas pesadas juntas (ex.: Frigate + testes)."
	case pico >= 75:
		it.Estado, it.Resumo = Atencao, fmt.Sprintf("CPU quente: chegou a %.0f °C.", pico)
		it.Recomendacao = "Ainda seguro, mas vale ver a ventilação e não deixar tarefas pesadas juntas."
	default:
		it.Estado, it.Resumo = OK, fmt.Sprintf("Temperatura boa: %.0f °C.", *a.TempCPU)
	}
	return it
}

func ligado(e Entrada) Item {
	if e.Computador == nil {
		return Item{}
	}
	dias := e.Computador.Agora.LigadoS / 86400
	it := Item{ID: "ligado", Area: areaPC, Titulo: "Tempo ligado", peso: 0.2}
	it.Prova = fmt.Sprintf("ligado há %.1f dias", dias)
	if dias > 30 {
		it.Estado, it.Resumo = Dica, fmt.Sprintf("Ligado há %.0f dias sem reiniciar.", dias)
		it.Recomendacao = "Reinicie quando puder: atualizações do sistema (inclusive de segurança) só valem depois de reiniciar."
		return it
	}
	it.Estado, it.Resumo = OK, fmt.Sprintf("Ligado há %s.", map[bool]string{true: fmt.Sprintf("%.0f h", dias*24), false: fmt.Sprintf("%.0f dias", dias)}[dias < 2])
	return it
}

func gb(mb int) float64 { return float64(mb) / 1024 }

// ---- ajudantes ----

// decimal com ponto que não faz parte de um endereço (8.8.8.8 fica como está).
var decimalPonto = regexp.MustCompile(`(^|[^\d.])(\d+)\.(\d+)([^\d.]|$)`)

// virgula troca "5.3" por "5,3" no texto que vai para a tela.
func virgula(s string) string {
	for {
		n := decimalPonto.ReplaceAllString(s, "${1}${2},${3}${4}")
		if n == s {
			return s
		}
		s = n
	}
}

func aberta(seg string) string {
	if seg == "" {
		return "aberta"
	}
	return seg
}

func s(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func media(v []float64) float64 {
	var t float64
	for _, x := range v {
		t += x
	}
	return t / float64(len(v))
}

func mediana(v []float64) float64 {
	if len(v) == 0 {
		return 0
	}
	c := slices.Clone(v)
	slices.Sort(c)
	if len(c)%2 == 1 {
		return c[len(c)/2]
	}
	return (c[len(c)/2-1] + c[len(c)/2]) / 2
}

func duracao(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d s", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d min", int(d.Minutes()))
	}
	return fmt.Sprintf("%d h %d min", int(d.Hours()), int(d.Minutes())%60)
}
