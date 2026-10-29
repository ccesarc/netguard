package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"netguard/internal/armazenamento"
	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
)

type Consultas interface {
	Historico(ctx context.Context, de, ate time.Time, passo time.Duration) ([]armazenamento.PontoHistorico, error)
	Disponibilidade(ctx context.Context, de, ate time.Time) (armazenamento.Disponibilidade, error)
	Incidentes(ctx context.Context, de, ate time.Time, limite int) ([]incidentes.Incidente, error)
}

type periodo struct {
	duracao time.Duration
	passo   time.Duration // tamanho de cada ponto do gráfico
}

var periodos = map[string]periodo{
	"1h":  {time.Hour, time.Minute},
	"6h":  {6 * time.Hour, 5 * time.Minute},
	"24h": {24 * time.Hour, 15 * time.Minute},
	"7d":  {7 * 24 * time.Hour, time.Hour},
	"30d": {30 * 24 * time.Hour, 6 * time.Hour},
}

func lerPeriodo(w http.ResponseWriter, r *http.Request, padrao string) (string, periodo, bool) {
	nome := r.URL.Query().Get("periodo")
	if nome == "" {
		nome = padrao
	}
	p, ok := periodos[nome]
	if !ok {
		erroJSON(w, http.StatusBadRequest, "periodo inválido (use 1h, 6h, 24h, 7d ou 30d)")
	}
	return nome, p, ok
}

type respHistorico struct {
	Periodo       string     `json:"periodo"`
	PassoS        int        `json:"passo_s"`
	T             []int64    `json:"t"` // início de cada ponto, em segundos (Unix)
	GatewayMS     []*float64 `json:"gateway_ms"`
	GatewayPerda  []*float64 `json:"gateway_perda"`
	InternetMS    []*float64 `json:"internet_ms"`
	InternetPerda []*float64 `json:"internet_perda"`
	DNSMS         []*float64 `json:"dns_ms"`
	Fora          []*float64 `json:"fora"` // fração dos ciclos fora no ponto
}

func (s *servidor) historico(w http.ResponseWriter, r *http.Request) {
	nome, p, ok := lerPeriodo(w, r, "1h")
	if !ok {
		return
	}
	ate := s.agora()
	de := ate.Add(-p.duracao).Truncate(p.passo)
	pts, err := s.Consultas.Historico(r.Context(), de, ate, p.passo)
	if err != nil {
		s.falhaInterna(w, "histórico", err)
		return
	}
	porHora := make(map[int64]armazenamento.PontoHistorico, len(pts))
	for _, pt := range pts {
		porHora[pt.Em.Unix()] = pt
	}
	// Todos os pontos do período, com nulo onde não houve medição: o gráfico
	// mostra o buraco em vez de ligar os pontos por cima dele.
	resp := respHistorico{Periodo: nome, PassoS: int(p.passo.Seconds())}
	for t := de; t.Before(ate); t = t.Add(p.passo) {
		pt := porHora[t.Unix()]
		resp.T = append(resp.T, t.Unix())
		resp.GatewayMS = append(resp.GatewayMS, arred(pt.GatewayMS, 1))
		resp.GatewayPerda = append(resp.GatewayPerda, arred(pt.GatewayPerda, 3))
		resp.InternetMS = append(resp.InternetMS, arred(pt.InternetMS, 1))
		resp.InternetPerda = append(resp.InternetPerda, arred(pt.InternetPerda, 3))
		resp.DNSMS = append(resp.DNSMS, arred(pt.DNSMS, 1))
		resp.Fora = append(resp.Fora, arred(pt.FracaoFora, 3))
	}
	escreverJSON(w, http.StatusOK, resp)
}

type respDisponibilidade struct {
	Disponivel bool    `json:"disponivel"`
	Motivo     string  `json:"motivo,omitempty"`
	Percentual float64 `json:"percentual"` // ciclos no ar ÷ ciclos medidos
	Cobertura  float64 `json:"cobertura"`  // parte do período com o monitor rodando
	Quedas     int     `json:"quedas"`
	TempoForaS float64 `json:"tempo_fora_s"`
	Ciclos     int     `json:"ciclos"`
}

func (s *servidor) disponibilidade(w http.ResponseWriter, r *http.Request) {
	resp := map[string]respDisponibilidade{}
	ate := s.agora()
	for _, nome := range []string{"24h", "7d", "30d"} {
		d, err := s.Consultas.Disponibilidade(r.Context(), ate.Add(-periodos[nome].duracao), ate)
		if err != nil {
			s.falhaInterna(w, "disponibilidade", err)
			return
		}
		resp[nome] = montarDisponibilidade(d, periodos[nome].duracao)
	}
	escreverJSON(w, http.StatusOK, resp)
}

func montarDisponibilidade(d armazenamento.Disponibilidade, periodo time.Duration) respDisponibilidade {
	if d.CiclosTotal == 0 {
		return respDisponibilidade{Motivo: "Sem medições neste período."}
	}
	return respDisponibilidade{
		Disponivel: true,
		Percentual: float64(d.CiclosNoAr) / float64(d.CiclosTotal),
		Cobertura:  min(1, d.SegundosCobertos/periodo.Seconds()),
		Quedas:     d.Quedas,
		TempoForaS: d.SegundosFora,
		Ciclos:     d.CiclosTotal,
	}
}

type respCausa struct {
	Causa diagnostico.Causa `json:"causa"`
	Texto string            `json:"texto"`
}

type respQueda struct {
	ID          int64       `json:"id"`
	Inicio      time.Time   `json:"inicio"`
	Fim         *time.Time  `json:"fim"`
	DuracaoS    float64     `json:"duracao_s"`
	EmAndamento bool        `json:"em_andamento"`
	Causa       respCausa   `json:"causa"`
	Causas      []respCausa `json:"causas"`
	Detalhe     string      `json:"detalhe,omitempty"`
}

func montarQueda(q incidentes.Incidente, agora time.Time) respQueda {
	r := respQueda{
		ID: q.ID, Inicio: q.Inicio, Fim: q.Fim, EmAndamento: q.Fim == nil, Detalhe: q.Detalhe,
		Causa: respCausa{q.Causa, diagnostico.Texto(q.Causa)},
	}
	fim := agora
	if q.Fim != nil {
		fim = *q.Fim
	}
	r.DuracaoS = fim.Sub(q.Inicio).Seconds()
	for _, c := range q.Causas {
		r.Causas = append(r.Causas, respCausa{c, diagnostico.Texto(c)})
	}
	return r
}

func (s *servidor) listarQuedas(w http.ResponseWriter, r *http.Request) {
	_, p, ok := lerPeriodo(w, r, "30d")
	if !ok {
		return
	}
	limite := 100
	if v := r.URL.Query().Get("limite"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 500 {
			erroJSON(w, http.StatusBadRequest, "limite inválido (1 a 500)")
			return
		}
		limite = n
	}
	agora := s.agora()
	lista, err := s.Consultas.Incidentes(r.Context(), agora.Add(-p.duracao), agora, limite)
	if err != nil {
		s.falhaInterna(w, "quedas", err)
		return
	}
	resp := make([]respQueda, 0, len(lista))
	for _, q := range lista {
		resp = append(resp, montarQueda(q, agora))
	}
	escreverJSON(w, http.StatusOK, map[string]any{"quedas": resp})
}

func (s *servidor) falhaInterna(w http.ResponseWriter, oque string, err error) {
	s.Log.Error("consulta falhou", "oque", oque, "erro", err)
	erroJSON(w, http.StatusServiceUnavailable, "não deu para ler o "+oque+" agora")
}

func arred(v *float64, casas int) *float64 {
	if v == nil {
		return nil
	}
	f := 1.0
	for range casas {
		f *= 10
	}
	x := float64(int64(*v*f+0.5)) / f
	return &x
}
