package wifi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Rede é uma rede Wi-Fi anunciada por perto (nome, canal e sinal são públicos:
// todo roteador anuncia). Nada além disso é coletado.
type Rede struct {
	SSID      string `json:"ssid"`
	BSSID     string `json:"-"` // usado só para reconhecer os rádios do próprio roteador
	Canal     int    `json:"canal"`
	FreqMHz   int    `json:"freq_mhz"`
	Sinal     int    `json:"sinal"`
	Seguranca string `json:"seguranca,omitempty"`
	Conectada bool   `json:"conectada"`
	Propria   bool   `json:"propria"` // do mesmo roteador da rede conectada (outra faixa)
}

func (r Rede) Faixa24() bool { return r.FreqMHz > 0 && r.FreqMHz < 3000 }
func (r Rede) Faixa5() bool  { return r.FreqMHz >= 4900 && r.FreqMHz < 5900 }

type Buscador interface {
	Buscar(ctx context.Context, iface string) ([]Rede, error)
}

// BuscadorNmcli pede uma busca nova ao NetworkManager e lê o resultado.
// Precisa da regra do polkit (scripts/permitir-busca-wifi.sh) quando ninguém
// está logado na tela. A busca pausa o Wi-Fi por ~1–2 s; por isso é espaçada.
type BuscadorNmcli struct{}

func (BuscadorNmcli) Buscar(ctx context.Context, iface string) ([]Rede, error) {
	env := []string{"LC_ALL=C", "PATH=/usr/bin:/bin"}
	args := []string{"dev", "wifi", "rescan"}
	if iface != "" {
		args = append(args, "ifname", iface)
	}
	cmd := exec.CommandContext(ctx, "nmcli", args...)
	cmd.Env = env
	if saida, err := cmd.CombinedOutput(); err != nil {
		if strings.Contains(string(saida), "not authorized") {
			return nil, errors.New("o sistema não autorizou a busca (falta a regra: scripts/permitir-busca-wifi.sh)")
		}
		return nil, fmt.Errorf("nmcli rescan: %s", strings.TrimSpace(string(saida)))
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(5 * time.Second): // o resultado da busca leva alguns segundos
	}
	args = []string{"-t", "-f", "IN-USE,BSSID,SSID,CHAN,FREQ,SIGNAL,SECURITY", "dev", "wifi", "list", "--rescan", "no"}
	if iface != "" {
		args = append(args, "ifname", iface)
	}
	cmd = exec.CommandContext(ctx, "nmcli", args...)
	cmd.Env = env
	saida, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("nmcli list: %w", err)
	}
	return analisarLista(string(saida)), nil
}

func analisarLista(saida string) []Rede {
	var redes []Rede
	var conectada string
	for _, linha := range strings.Split(strings.TrimSpace(saida), "\n") {
		c := campos(linha)
		if len(c) < 7 {
			continue
		}
		r := Rede{
			BSSID: strings.ToUpper(c[1]), SSID: c[2], Canal: inteiro(c[3]), FreqMHz: inteiro(c[4]),
			Sinal: min(100, max(0, inteiro(c[5]))), Seguranca: c[6], Conectada: c[0] == "*",
		}
		if r.Conectada {
			conectada = r.BSSID
		}
		redes = append(redes, r)
	}
	// O mesmo roteador usa endereços quase iguais nas duas faixas (só muda o último byte).
	if len(conectada) >= 14 {
		for i := range redes {
			redes[i].Propria = len(redes[i].BSSID) >= 14 && redes[i].BSSID[:14] == conectada[:14]
		}
	}
	sort.SliceStable(redes, func(i, j int) bool { return redes[i].Sinal > redes[j].Sinal })
	return redes
}

// ---- análise de canal ----

type Ocupacao struct {
	Canal     int      `json:"canal"`
	Redes     []string `json:"redes"`      // vizinhos neste canal (nome e sinal)
	MaisForte int      `json:"mais_forte"` // sinal do vizinho mais forte, 0 = livre
	Ocupacao  float64  `json:"ocupacao"`   // disputa estimada (0 = livre)
	Proprio   bool     `json:"proprio"`    // canal em uso pelo teu roteador
	Candidato bool     `json:"candidato"`  // canal que vale recomendar
	DFS       bool     `json:"dfs,omitempty"`
}

// Candidato é um canal (ou bloco, no 5 GHz) comparado com os outros, com o porquê.
type Candidato struct {
	Rotulo  string    `json:"rotulo"` // "canal 6" ou "canais 36–48"
	Canal   int       `json:"canal"`
	Disputa string    `json:"disputa"` // pouca, média ou muita
	Pontos  float64   `json:"pontos"`
	Motivo  string    `json:"motivo"`
	Atual   bool      `json:"atual"`
	Melhor  bool      `json:"melhor"`
	DFS     bool      `json:"dfs,omitempty"`
	Redes   []Vizinho `json:"redes"` // no mesmo canal (ou bloco), do mais forte ao mais fraco
	Perto   []Vizinho `json:"perto"` // em canal que se sobrepõe (só 2,4 GHz)
}

type Vizinho struct {
	Nome  string `json:"nome"`
	Sinal int    `json:"sinal"`
	Nivel string `json:"nivel"` // forte, média ou fraca
	Canal int    `json:"canal"`
}

func vizinhosDe(rs []Rede) []Vizinho {
	ordenadas := append([]Rede{}, rs...)
	sort.SliceStable(ordenadas, func(i, j int) bool { return ordenadas[i].Sinal > ordenadas[j].Sinal })
	v := make([]Vizinho, len(ordenadas))
	for i, r := range ordenadas {
		v[i] = Vizinho{Nome: nomeOuOculta(r.SSID), Sinal: r.Sinal, Nivel: nivel(r.Sinal), Canal: r.Canal}
	}
	return v
}

type Faixa struct {
	Nome        string      `json:"nome"` // "2,4 GHz" ou "5 GHz"
	TuaRede     string      `json:"tua_rede,omitempty"`
	CanalAtual  int         `json:"canal_atual"` // 0 = teu roteador não usa esta faixa (ou não apareceu)
	Recomendado int         `json:"recomendado"`
	Mudar       bool        `json:"mudar"`
	Texto       string      `json:"texto"`
	Canais      []Ocupacao  `json:"canais"`
	Vizinhos    int         `json:"vizinhos"`
	Comparacao  []Candidato `json:"comparacao"` // do melhor para o pior
}

const forte = 60 // sinal a partir do qual um vizinho "grita" no canal

func disputa(p float64) string {
	switch {
	case p < 0.25:
		return "pouca"
	case p < 0.8: // uma rede a ~90% já passa disso; três a ~40% ficam abaixo
		return "média"
	}
	return "muita"
}

// motivo explica a disputa: quais redes, com o sinal e o peso de cada uma.
// mesmas = no mesmo canal (ou bloco); perto = em canais que se sobrepõem (só 2,4 GHz).
func motivo(mesmas, perto []Rede) string {
	if len(mesmas) == 0 && len(perto) == 0 {
		return "livre de vizinhos"
	}
	var partes []string
	if len(mesmas) > 0 {
		ordenadas := append([]Rede{}, mesmas...)
		sort.SliceStable(ordenadas, func(i, j int) bool { return ordenadas[i].Sinal > ordenadas[j].Sinal })
		var itens []string
		for i, r := range ordenadas {
			if i == 3 {
				itens = append(itens, fmt.Sprintf("+%d", len(ordenadas)-3))
				break
			}
			itens = append(itens, fmt.Sprintf("%s %d%% (%s)", nomeOuOculta(r.SSID), r.Sinal, nivel(r.Sinal)))
		}
		partes = append(partes, fmt.Sprintf("%d rede%s: %s", len(mesmas), plural(len(mesmas)), strings.Join(itens, ", ")))
	}
	if len(perto) > 0 {
		maior := 0
		for _, r := range perto {
			maior = max(maior, r.Sinal)
		}
		partes = append(partes, fmt.Sprintf("mais %d em canal vizinho que se sobrepõe (até %d%%)", len(perto), maior))
	}
	return strings.Join(partes, "; ")
}

func nivel(sinal int) string {
	switch {
	case sinal >= forte:
		return "forte"
	case sinal >= 40:
		return "média"
	}
	return "fraca"
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func ordenar(cs []Candidato) []Candidato {
	sort.SliceStable(cs, func(i, j int) bool { return cs[i].Pontos < cs[j].Pontos })
	for i := range cs {
		cs[i].Melhor = i == 0 || cs[i].Pontos == cs[0].Pontos
	}
	return cs
}

// peso: rede forte atrapalha muito mais que rede fraca.
func peso(sinal int) float64 { s := float64(sinal) / 100; return s * s }

// Analisar24 considera canais de 20 MHz: dois canais se atrapalham se estão a
// menos de 5 de distância. Só 1, 6 e 11 não se sobrepõem — são os recomendados.
func Analisar24(redes []Rede) Faixa {
	f := Faixa{Nome: "2,4 GHz"}
	vizinhos := filtrar(redes, func(r Rede) bool { return r.Faixa24() && !r.Propria && !r.Conectada })
	for _, r := range redes {
		if r.Faixa24() && (r.Propria || r.Conectada) {
			f.CanalAtual, f.TuaRede = r.Canal, r.SSID
			break
		}
	}
	f.Vizinhos = len(vizinhos)
	pontos := func(c int) float64 {
		var p float64
		for _, v := range vizinhos {
			d := c - v.Canal
			if d < 0 {
				d = -d
			}
			p += peso(v.Sinal) * max(0, 1-float64(d)/5)
		}
		return p
	}
	for c := 1; c <= 13; c++ {
		o := Ocupacao{Canal: c, Ocupacao: arred(pontos(c)), Proprio: c == f.CanalAtual, Candidato: c == 1 || c == 6 || c == 11}
		preencher(&o, vizinhos, func(r Rede) bool { return r.Canal == c })
		f.Canais = append(f.Canais, o)
	}
	melhor := 1
	for _, c := range []int{6, 11} {
		if pontos(c) < pontos(melhor) {
			melhor = c
		}
	}
	f.Recomendado = melhor
	cands := []int{1, 6, 11}
	if f.CanalAtual != 0 && f.CanalAtual != 1 && f.CanalAtual != 6 && f.CanalAtual != 11 {
		cands = append(cands, f.CanalAtual)
	}
	for _, c := range cands {
		var mesmas, perto []Rede
		for _, v := range vizinhos {
			switch d := abs(c - v.Canal); {
			case d == 0:
				mesmas = append(mesmas, v)
			case d < 5:
				perto = append(perto, v)
			}
		}
		p := arred(pontos(c))
		f.Comparacao = append(f.Comparacao, Candidato{Rotulo: fmt.Sprintf("canal %d", c), Canal: c, Pontos: p,
			Disputa: disputa(p), Motivo: motivo(mesmas, perto), Atual: c == f.CanalAtual,
			Redes: vizinhosDe(mesmas), Perto: vizinhosDe(perto)})
	}
	f.Comparacao = ordenar(f.Comparacao)
	f.Mudar, f.Texto = decidir(f, pontos(f.CanalAtual), pontos(melhor), vizinhos)
	return f
}

// Blocos de 80 MHz do 5 GHz no Brasil. Os do meio (DFS) dividem espaço com
// radar: o roteador pode trocar de canal sozinho, e nem todo aparelho usa.
var blocos5 = []struct {
	de, ate int
	dfs     bool
}{{36, 48, false}, {52, 64, true}, {100, 112, true}, {116, 128, true}, {132, 144, true}, {149, 161, false}}

func bloco5(canal int) int {
	for i, b := range blocos5 {
		if canal >= b.de && canal <= b.ate {
			return i
		}
	}
	return -1
}

// Analisar5 considera blocos de 80 MHz (o comum em roteador Wi-Fi 6): redes no
// mesmo bloco disputam o ar, mesmo em canais "diferentes".
func Analisar5(redes []Rede) Faixa {
	f := Faixa{Nome: "5 GHz"}
	vizinhos := filtrar(redes, func(r Rede) bool { return r.Faixa5() && !r.Propria && !r.Conectada })
	for _, r := range redes {
		if r.Faixa5() && (r.Propria || r.Conectada) {
			f.CanalAtual, f.TuaRede = r.Canal, r.SSID
			break
		}
	}
	f.Vizinhos = len(vizinhos)
	pontosBloco := func(b int) float64 {
		var p float64
		for _, v := range vizinhos {
			if bloco5(v.Canal) == b {
				p += peso(v.Sinal)
			}
		}
		return p
	}
	atual := bloco5(f.CanalAtual)
	for i, b := range blocos5 {
		for c := b.de; c <= b.ate; c += 4 {
			o := Ocupacao{Canal: c, Ocupacao: arred(pontosBloco(i)), Proprio: c == f.CanalAtual, Candidato: !b.dfs, DFS: b.dfs}
			preencher(&o, vizinhos, func(r Rede) bool { return r.Canal == c })
			f.Canais = append(f.Canais, o)
		}
	}
	melhor := 0
	for i, b := range blocos5 {
		if !b.dfs && pontosBloco(i) < pontosBloco(melhor) {
			melhor = i
		}
	}
	f.Recomendado = blocos5[melhor].de
	if atual >= 0 && pontosBloco(atual) <= pontosBloco(melhor) {
		f.Recomendado = f.CanalAtual // o atual já está entre os melhores
	}
	for i, b := range blocos5 {
		if b.dfs && i != atual {
			continue
		}
		var mesmas []Rede
		for _, v := range vizinhos {
			if bloco5(v.Canal) == i {
				mesmas = append(mesmas, v)
			}
		}
		p := arred(pontosBloco(i))
		canal := b.de
		if i == atual {
			canal = f.CanalAtual
		}
		f.Comparacao = append(f.Comparacao, Candidato{Rotulo: fmt.Sprintf("canais %d–%d", b.de, b.ate), Canal: canal,
			Pontos: p, Disputa: disputa(p), Motivo: motivo(mesmas, nil), Atual: i == atual, DFS: b.dfs,
			Redes: vizinhosDe(mesmas), Perto: []Vizinho{}})
	}
	f.Comparacao = ordenar(f.Comparacao)
	var pAtual float64
	if atual >= 0 {
		pAtual = pontosBloco(atual)
	}
	f.Mudar, f.Texto = decidir(f, pAtual, pontosBloco(melhor), vizinhos)
	return f
}

// decidir só recomenda trocar quando a melhora é clara: trocar de canal derruba
// todo mundo do Wi-Fi por alguns segundos.
func decidir(f Faixa, atual, melhor float64, vizinhos []Rede) (bool, string) {
	if f.CanalAtual == 0 {
		if len(vizinhos) == 0 {
			return false, "Nenhuma rede nesta faixa por perto."
		}
		return false, fmt.Sprintf("Teu roteador não apareceu nesta faixa. Se usar, o canal mais livre é o %d.", f.Recomendado)
	}
	var dividem []Rede
	for _, v := range vizinhos {
		if f.Nome == "2,4 GHz" && abs(v.Canal-f.CanalAtual) < 5 || f.Nome == "5 GHz" && bloco5(v.Canal) == bloco5(f.CanalAtual) {
			dividem = append(dividem, v)
		}
	}
	sort.SliceStable(dividem, func(i, j int) bool { return dividem[i].Sinal > dividem[j].Sinal })
	divide := []string{}
	for i, v := range dividem {
		if i == 3 {
			divide = append(divide, fmt.Sprintf("mais %d", len(dividem)-3))
			break
		}
		divide = append(divide, fmt.Sprintf("%s (%d%%)", nomeOuOculta(v.SSID), v.Sinal))
	}
	if atual-melhor > 0.15 && melhor < atual*0.6 && f.Recomendado != f.CanalAtual {
		return true, fmt.Sprintf("Vale trocar para o canal %d: no %d o ar é dividido com %s. Troca-se no painel do roteador.",
			f.Recomendado, f.CanalAtual, juntar(divide))
	}
	if len(divide) == 0 {
		return false, fmt.Sprintf("Canal %d está livre de vizinhos. Não mexa.", f.CanalAtual)
	}
	if atual < 0.2 {
		if len(divide) == 1 {
			return false, fmt.Sprintf("Canal %d está bom: só %s, fraco, divide o ar. Não mexa.", f.CanalAtual, divide[0])
		}
		return false, fmt.Sprintf("Canal %d está bom: só %s, fracos, dividem o ar. Não mexa.", f.CanalAtual, juntar(divide))
	}
	return false, fmt.Sprintf("Canal %d divide o ar com %s, mas os outros não estão melhores. Não mexa.", f.CanalAtual, juntar(divide))
}

func preencher(o *Ocupacao, vizinhos []Rede, no func(Rede) bool) {
	for _, v := range vizinhos {
		if no(v) {
			o.Redes = append(o.Redes, fmt.Sprintf("%s (%d%%)", nomeOuOculta(v.SSID), v.Sinal))
			o.MaisForte = max(o.MaisForte, v.Sinal)
		}
	}
}

func nomeOuOculta(s string) string {
	if s == "" {
		return "rede oculta"
	}
	return s
}

func filtrar(redes []Rede, ok func(Rede) bool) []Rede {
	var r []Rede
	for _, x := range redes {
		if ok(x) {
			r = append(r, x)
		}
	}
	return r
}

func juntar(itens []string) string {
	if len(itens) <= 1 {
		return strings.Join(itens, "")
	}
	return strings.Join(itens[:len(itens)-1], ", ") + " e " + itens[len(itens)-1]
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func arred(x float64) float64 { return float64(int64(x*100+0.5)) / 100 }

// ---- vigia da vizinhança ----

type Vizinhanca struct {
	Em     time.Time   `json:"em"`
	Redes  []Rede      `json:"redes"`  // as da última busca
	Vistas []RedeVista `json:"vistas"` // todas as vistas nas últimas buscas, com o sinal de cada
	F24    Faixa       `json:"faixa_24"`
	F5     Faixa       `json:"faixa_5"`
	Buscas int         `json:"buscas"` // quantas buscas a recomendação considerou
}

// RedeVista é uma rede que apareceu em pelo menos uma das últimas buscas.
type RedeVista struct {
	Rede
	SinalMedio int `json:"sinal_medio"` // média das vezes em que foi ouvida
	Vezes      int `json:"vezes"`       // em quantas buscas apareceu
	Buscas     int `json:"buscas"`      // quantas buscas foram consideradas
}

// juntarBuscas une duas escutas da mesma busca; fica o sinal mais forte de cada rede.
func juntarBuscas(a, b []Rede) []Rede {
	por := map[string]int{}
	r := append([]Rede{}, a...)
	for i, x := range r {
		por[chaveRede(x)] = i
	}
	for _, x := range b {
		if i, ok := por[chaveRede(x)]; ok {
			if x.Sinal > r[i].Sinal {
				r[i].Sinal = x.Sinal
			}
			r[i].Conectada = r[i].Conectada || x.Conectada
			continue
		}
		por[chaveRede(x)] = len(r)
		r = append(r, x)
	}
	sort.SliceStable(r, func(i, j int) bool { return r[i].Sinal > r[j].Sinal })
	return r
}

func chaveRede(r Rede) string {
	if r.BSSID != "" {
		return r.BSSID
	}
	return r.SSID + "|" + strconv.Itoa(r.Canal)
}

// reunir lista toda rede vista nas últimas buscas, com o sinal médio de quando
// foi ouvida e em quantas apareceu. Uma busca ruim não encolhe a lista.
func reunir(buscas [][]Rede) []RedeVista {
	por := map[string]*RedeVista{}
	var ordem []string
	soma := map[string]int{}
	for _, b := range buscas {
		for _, r := range b {
			k := chaveRede(r)
			x, ok := por[k]
			if !ok {
				x = &RedeVista{}
				por[k] = x
				ordem = append(ordem, k)
			}
			conectada := x.Conectada || r.Conectada
			x.Rede = r // a leitura mais recente
			x.Conectada = conectada
			x.Vezes++
			soma[k] += r.Sinal
		}
	}
	lista := make([]RedeVista, 0, len(ordem))
	for _, k := range ordem {
		x := por[k]
		x.SinalMedio = (soma[k] + x.Vezes/2) / x.Vezes
		x.Buscas = len(buscas)
		lista = append(lista, *x)
	}
	sort.SliceStable(lista, func(i, j int) bool {
		ti, tj := lista[i].Propria || lista[i].Conectada, lista[j].Propria || lista[j].Conectada
		if ti != tj {
			return ti // as tuas primeiro
		}
		return lista[i].SinalMedio > lista[j].SinalMedio
	})
	return lista
}

// Quantas buscas entram na recomendação (a cada 30 min = ~3 h). Uma busca só
// é instável: rede some e volta, e mandar trocar o canal por isso seria ruim.
const buscasConsideradas = 6

// mesclar junta as últimas buscas: o sinal de cada rede vira a média com zero
// nas buscas em que ela não apareceu (rede que aparece pouco pesa pouco).
func mesclar(buscas [][]Rede) []Rede {
	if len(buscas) == 0 {
		return nil
	}
	type soma struct {
		rede  Rede
		total int
	}
	por := map[string]*soma{}
	var ordem []string
	for _, b := range buscas {
		for _, r := range b {
			chave := r.BSSID
			if chave == "" {
				chave = r.SSID + "|" + strconv.Itoa(r.Canal)
			}
			x, ok := por[chave]
			if !ok {
				x = &soma{}
				por[chave] = x
				ordem = append(ordem, chave)
			}
			x.rede = r // fica a leitura mais recente (canal pode ter mudado)
			x.total += r.Sinal
		}
	}
	var r []Rede
	for _, k := range ordem {
		x := por[k]
		rede := x.rede
		rede.Sinal = (x.total + len(buscas)/2) / len(buscas)
		if rede.Sinal > 0 {
			r = append(r, rede)
		}
	}
	sort.SliceStable(r, func(i, j int) bool { return r[i].Sinal > r[j].Sinal })
	return r
}

var (
	ErrBuscando  = errors.New("já tem uma busca rodando")
	ErrBuscaCedo = errors.New("espere 1 minuto entre uma busca e outra")
)

const intervaloVizinhanca = 30 * time.Minute

// Vigia busca as redes vizinhas a cada 30 min (e quando pedem pela tela).
type Vigia struct {
	buscador Buscador
	iface    func() (nome, tipo string)
	ocupado  func() bool // não busca durante o teste de velocidade
	log      *slog.Logger

	mu        sync.Mutex
	buscando  bool
	ultima    *Vizinhanca
	tentada   time.Time
	motivo    string
	repetir   bool              // escuta duas vezes por busca (falso nos testes, para não esperar)
	proprias  map[bool]lembrada // faixa 2,4 GHz? → último rádio do teu roteador visto
	historico [][]Rede          // últimas buscas, para a média
}

func NovoVigia(b Buscador, iface func() (string, string), ocupado func() bool, log *slog.Logger) *Vigia {
	return &Vigia{buscador: b, iface: iface, ocupado: ocupado, log: log, motivo: "Aguardando a primeira busca.", repetir: true}
}

func (v *Vigia) Ultima() (*Vizinhanca, string, bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.ultima, v.motivo, v.buscando
}

func (v *Vigia) Rodar(ctx context.Context) {
	time.Sleep(20 * time.Second) // deixa o resto subir antes
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		v.mu.Lock()
		vez := !v.buscando && time.Since(v.tentada) >= intervaloVizinhanca && !v.ocupado()
		if vez {
			v.buscando, v.tentada = true, time.Now()
		}
		v.mu.Unlock()
		if vez {
			v.buscar(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// BuscandoDesde: quando começou a busca em andamento (zero se não há).
func (v *Vigia) BuscandoDesde() time.Time {
	v.mu.Lock()
	defer v.mu.Unlock()
	if !v.buscando {
		return time.Time{}
	}
	return v.tentada
}

// Buscando diz se uma busca está em andamento (o monitor descarta esse ciclo).
func (v *Vigia) Buscando() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.buscando
}

func (v *Vigia) BuscarAgora(ctx context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.buscando {
		return ErrBuscando
	}
	if time.Since(v.tentada) < time.Minute {
		return ErrBuscaCedo
	}
	v.buscando, v.tentada = true, time.Now()
	go v.buscar(context.WithoutCancel(ctx))
	return nil
}

func (v *Vigia) buscar(ctx context.Context) {
	defer func() {
		v.mu.Lock()
		v.buscando = false
		v.mu.Unlock()
	}()
	nome, tipo := v.iface()
	if tipo != "wifi" {
		v.definir(nil, "Este aparelho não está usando Wi-Fi: não dá para ver as redes por perto.")
		return
	}
	bctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	// Cada busca só "ouve" parte das redes (fraca ou longe às vezes não é ouvida
	// naquele instante): escuta duas vezes seguidas e junta.
	redes, err := v.buscador.Buscar(bctx, nome)
	if err == nil && v.repetir {
		select {
		case <-bctx.Done():
		case <-time.After(3 * time.Second):
			if mais, err2 := v.buscador.Buscar(bctx, nome); err2 == nil {
				redes = juntarBuscas(redes, mais)
			}
		}
	}
	if err != nil {
		v.log.Warn("busca de redes Wi-Fi falhou", "erro", err)
		v.mu.Lock()
		if v.ultima == nil {
			v.motivo = "Não deu para buscar as redes: " + err.Error()
		}
		v.mu.Unlock()
		return
	}
	redes = v.completarProprias(redes)
	v.mu.Lock()
	v.historico = append(v.historico, redes)
	if len(v.historico) > buscasConsideradas {
		v.historico = v.historico[len(v.historico)-buscasConsideradas:]
	}
	media := mesclar(v.historico)
	vistas := reunir(v.historico)
	n := len(v.historico)
	v.mu.Unlock()
	viz := &Vizinhanca{Em: time.Now(), Redes: redes, Vistas: vistas, F24: Analisar24(media), F5: Analisar5(media), Buscas: n}
	v.definir(viz, "")
	v.log.Info("redes Wi-Fi por perto", "nesta_busca", len(redes), "nas_ultimas", len(vistas))
}

// completarProprias: nem toda busca recebe resposta de todos os rádios. Se o
// teu roteador sumiu de uma faixa nesta busca, usa o que foi visto nas últimas 24 h.
func (v *Vigia) completarProprias(redes []Rede) []Rede {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.proprias == nil {
		v.proprias = map[bool]lembrada{}
	}
	tem := map[bool]bool{}
	for _, r := range redes {
		if (r.Propria || r.Conectada) && (r.Faixa24() || r.Faixa5()) {
			v.proprias[r.Faixa24()] = lembrada{r, time.Now()}
			tem[r.Faixa24()] = true
		}
	}
	for faixa24, l := range v.proprias {
		if !tem[faixa24] && time.Since(l.em) < 24*time.Hour {
			r := l.rede
			r.Propria, r.Conectada = true, false
			redes = append(redes, r)
		}
	}
	return redes
}

type lembrada struct {
	rede Rede
	em   time.Time
}

func (v *Vigia) definir(viz *Vizinhanca, motivo string) {
	v.mu.Lock()
	v.ultima, v.motivo = viz, motivo
	v.mu.Unlock()
}
