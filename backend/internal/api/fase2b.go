package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"reflect"
	"strings"
	"time"

	"netguard/internal/ajustes"
	"netguard/internal/armazenamento"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

type GerenteAjustes interface {
	Atuais() ajustes.Ajustes
	Padrao() ajustes.Ajustes
	Salvar(ctx context.Context, a ajustes.Ajustes, por string) error
}

type Velocimetro interface {
	Estado() velocidade.Estado
	TestarAgora() error
}

type FonteWifi interface {
	Ultima() (*wifi.Amostra, string)
}

type VigiaVizinhanca interface {
	Ultima() (*wifi.Vizinhanca, string, bool)
	BuscandoDesde() time.Time
	BuscarAgora(ctx context.Context) error
}

type ConsultasFase2B interface {
	TestesVelocidade(ctx context.Context, de, ate time.Time) ([]velocidade.Resultado, error)
	BytesTesteVelocidade(ctx context.Context, de, ate time.Time) (int64, error)
	HistoricoWifi(ctx context.Context, de, ate time.Time, passo time.Duration) ([]armazenamento.PontoWifi, error)
}

// ---- velocidade ----

func (s *servidor) velocidade(w http.ResponseWriter, r *http.Request) {
	_, p, ok := lerPeriodo(w, r, "30d")
	if !ok {
		return
	}
	agora := s.agora()
	testes, err := s.Consultas2B.TestesVelocidade(r.Context(), agora.Add(-p.duracao), agora)
	if err != nil {
		s.falhaInterna(w, "teste de velocidade", err)
		return
	}
	gasto, err := s.Consultas2B.BytesTesteVelocidade(r.Context(), agora.AddDate(0, 0, -30), agora)
	if err != nil {
		s.falhaInterna(w, "teste de velocidade", err)
		return
	}
	aj := s.Ajustes.Atuais()
	if testes == nil {
		testes = []velocidade.Resultado{}
	}
	escreverJSON(w, http.StatusOK, map[string]any{
		"estado":            s.Velocidade.Estado(),
		"testes":            testes,
		"gasto_30d_mb":      gasto >> 20,
		"estimativa_mes_mb": aj.ConsumoMensalMB(),
		"config":            aj.Velocidade,
	})
}

func (s *servidor) testarVelocidade(w http.ResponseWriter, r *http.Request) {
	err := s.Velocidade.TestarAgora()
	switch {
	case errors.Is(err, velocidade.ErrRodando):
		erroJSON(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, velocidade.ErrCedo):
		w.Header().Set("Retry-After", "120")
		erroJSON(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.falhaInterna(w, "teste de velocidade", err)
		return
	}
	u := usuarioDe(r)
	s.auditar(r, &u, "teste_velocidade", "", nil)
	escreverJSON(w, http.StatusAccepted, map[string]any{"estado": s.Velocidade.Estado()})
}

// ---- Wi-Fi ----

type respHistoricoWifi struct {
	PassoS   int        `json:"passo_s"`
	T        []int64    `json:"t"`
	Sinal    []*float64 `json:"sinal"`
	SinalMin []*float64 `json:"sinal_min"`
	Taxa     []*float64 `json:"taxa_mbps"`
}

func (s *servidor) wifi(w http.ResponseWriter, r *http.Request) {
	_, p, ok := lerPeriodo(w, r, "24h")
	if !ok {
		return
	}
	resp := map[string]any{}
	if a, motivo := s.Wifi.Ultima(); a != nil {
		resp["atual"] = map[string]any{"disponivel": true, "amostra": a}
	} else {
		resp["atual"] = indisponivel(motivo)
	}
	ate := s.agora()
	de := ate.Add(-p.duracao).Truncate(p.passo)
	pts, err := s.Consultas2B.HistoricoWifi(r.Context(), de, ate, p.passo)
	if err != nil {
		s.falhaInterna(w, "histórico do Wi-Fi", err)
		return
	}
	porHora := map[int64]armazenamento.PontoWifi{}
	for _, pt := range pts {
		porHora[pt.Em.Unix()] = pt
	}
	h := respHistoricoWifi{PassoS: int(p.passo.Seconds())}
	for t := de; t.Before(ate); t = t.Add(p.passo) {
		pt := porHora[t.Unix()]
		h.T = append(h.T, t.Unix())
		h.Sinal = append(h.Sinal, arred(pt.Sinal, 0))
		h.SinalMin = append(h.SinalMin, arred(pt.SinalMin, 0))
		h.Taxa = append(h.Taxa, arred(pt.TaxaMbps, 0))
	}
	resp["historico"] = h
	escreverJSON(w, http.StatusOK, resp)
}

// ---- configurações ----

func (s *servidor) respConfiguracao() map[string]any {
	a := s.Ajustes.Atuais()
	return map[string]any{
		"ajustes":           a,
		"padrao":            s.Ajustes.Padrao(),
		"estimativa_mes_mb": a.ConsumoMensalMB(),
	}
}

func (s *servidor) lerConfiguracao(w http.ResponseWriter, r *http.Request) {
	escreverJSON(w, http.StatusOK, s.respConfiguracao())
}

func (s *servidor) salvarConfiguracao(w http.ResponseWriter, r *http.Request) {
	var novo ajustes.Ajustes
	if !lerJSON(w, r, &novo) {
		return
	}
	if err := novo.Validar(); err != nil {
		escreverJSON(w, http.StatusBadRequest, map[string]any{
			"erro":  "Algumas configurações não são válidas.",
			"erros": strings.Split(err.Error(), "\n"),
		})
		return
	}
	antes := s.Ajustes.Atuais()
	u := usuarioDe(r)
	if err := s.Ajustes.Salvar(r.Context(), novo, u.Nome); err != nil {
		s.falhaInterna(w, "salvamento das configurações", err)
		return
	}
	s.auditar(r, &u, "configuracao_alterada", "", diferencas(antes, novo))
	escreverJSON(w, http.StatusOK, s.respConfiguracao())
}

// diferencas lista só os campos que mudaram, com o valor antigo e o novo.
func diferencas(antes, depois ajustes.Ajustes) map[string]any {
	var a, d map[string]any
	ba, _ := json.Marshal(antes)
	bd, _ := json.Marshal(depois)
	json.Unmarshal(ba, &a)
	json.Unmarshal(bd, &d)
	r := map[string]any{}
	for k, v := range d {
		if !reflect.DeepEqual(a[k], v) {
			r[k] = map[string]any{"antes": a[k], "depois": v}
		}
	}
	return r
}

// ---- Wi-Fi da vizinhança ----

func (s *servidor) vizinhanca(w http.ResponseWriter, r *http.Request) {
	viz, motivo, buscando := s.Vizinhanca.Ultima()
	var desde *time.Time
	if t := s.Vizinhanca.BuscandoDesde(); !t.IsZero() {
		desde = &t
	}
	if viz == nil {
		escreverJSON(w, http.StatusOK, map[string]any{"disponivel": false, "motivo": motivo, "buscando": buscando, "buscando_desde": desde})
		return
	}
	escreverJSON(w, http.StatusOK, map[string]any{"disponivel": true, "buscando": buscando, "buscando_desde": desde, "vizinhanca": viz})
}

func (s *servidor) buscarVizinhanca(w http.ResponseWriter, r *http.Request) {
	err := s.Vizinhanca.BuscarAgora(r.Context())
	switch {
	case errors.Is(err, wifi.ErrBuscando):
		erroJSON(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, wifi.ErrBuscaCedo):
		w.Header().Set("Retry-After", "60")
		erroJSON(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.falhaInterna(w, "busca de redes", err)
		return
	}
	u := usuarioDe(r)
	s.auditar(r, &u, "busca_wifi", "", nil)
	w.WriteHeader(http.StatusAccepted)
}
