package api

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"netguard/internal/armazenamento"
	"netguard/internal/dispositivos"
)

type ConsultasDispositivos interface {
	Dispositivos(ctx context.Context) ([]armazenamento.Dispositivo, error)
	Dispositivo(ctx context.Context, id int64) (armazenamento.Dispositivo, error)
	Aparicoes(ctx context.Context, id int64, limite int) ([]armazenamento.Aparicao, error)
	AtualizarDispositivo(ctx context.Context, id int64, m armazenamento.Mudanca) (armazenamento.Dispositivo, error)
	AparelhosNovos(ctx context.Context, de, ate time.Time) ([]armazenamento.Dispositivo, error)
	Movimentos(ctx context.Context, de, ate time.Time, janela time.Duration, limite int) ([]armazenamento.Movimento, error)
	Presencas(ctx context.Context, de, ate time.Time, janela time.Duration) (map[int64]armazenamento.Presenca, error)
}

type Descoberta interface {
	Estado() dispositivos.Estado
	VarrerAgora(ctx context.Context) error
}

type respDispositivo struct {
	armazenamento.Dispositivo
	Nome       string                  `json:"nome"`    // apelido, ou o nome descoberto, ou o fabricante
	NaRede     bool                    `json:"na_rede"` // visto nos últimos 10 min
	Presenca7d *armazenamento.Presenca `json:"presenca_7d,omitempty"`
}

func (s *servidor) montarDispositivo(d armazenamento.Dispositivo) respDispositivo {
	r := respDispositivo{Dispositivo: d, NaRede: s.agora().Sub(d.UltimoVisto) < dispositivos.Janela}
	switch {
	case d.Apelido != "":
		r.Nome = d.Apelido
	case d.EsteAparelho:
		r.Nome = "Este aparelho (NetGuard)"
	case d.NomeDescoberto != "":
		r.Nome = d.NomeDescoberto
	case d.Fabricante != "":
		r.Nome = "Aparelho " + nomeCurto(d.Fabricante)
	case d.MACAleatorio:
		r.Nome = "Aparelho sem nome (MAC aleatório)"
	default:
		r.Nome = "Aparelho sem nome"
	}
	return r
}

// nomeCurto tira o sufixo de empresa: "Amazon Technologies Inc." → "Amazon".
func nomeCurto(f string) string {
	for _, suf := range []string{",", " Inc", " Corporation", " Corp", " Co.", " Co ", " Ltd", " LLC", " GmbH", " S.A", " Technologies", " Electronics", " Communications", " International"} {
		if i := strings.Index(f, suf); i > 0 {
			f = f[:i]
		}
	}
	return strings.TrimSpace(f)
}

type resumoDispositivos struct {
	Total  int `json:"total"`
	NaRede int `json:"na_rede"`
	Novos  int `json:"novos"` // pedindo atenção do dono
}

func (s *servidor) listaDispositivos(ctx context.Context) ([]respDispositivo, resumoDispositivos, error) {
	ds, err := s.Dispositivos.Dispositivos(ctx)
	if err != nil {
		return nil, resumoDispositivos{}, err
	}
	agora := s.agora()
	pres, err := s.Dispositivos.Presencas(ctx, agora.Add(-7*24*time.Hour), agora, dispositivos.Janela)
	if err != nil {
		return nil, resumoDispositivos{}, err
	}
	lista := make([]respDispositivo, 0, len(ds))
	var res resumoDispositivos
	for _, d := range ds {
		r := s.montarDispositivo(d)
		if p, ok := pres[d.ID]; ok {
			r.Presenca7d = &p
		}
		lista = append(lista, r)
		res.Total++
		if r.NaRede {
			res.NaRede++
		}
		if r.AlertarNovo {
			res.Novos++
		}
	}
	return lista, res, nil
}

func (s *servidor) listarDispositivos(w http.ResponseWriter, r *http.Request) {
	lista, res, err := s.listaDispositivos(r.Context())
	if err != nil {
		s.falhaInterna(w, "lista de aparelhos", err)
		return
	}
	escreverJSON(w, http.StatusOK, map[string]any{"estado": s.Descoberta.Estado(), "resumo": res, "dispositivos": lista})
}

func idDe(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		erroJSON(w, http.StatusBadRequest, "id inválido")
		return 0, false
	}
	return id, true
}

func (s *servidor) detalheDispositivo(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r)
	if !ok {
		return
	}
	d, err := s.Dispositivos.Dispositivo(r.Context(), id)
	if errors.Is(err, armazenamento.ErrNaoEncontrado) {
		erroJSON(w, http.StatusNotFound, "aparelho não encontrado")
		return
	}
	if err != nil {
		s.falhaInterna(w, "aparelho", err)
		return
	}
	ap, err := s.Dispositivos.Aparicoes(r.Context(), id, 50)
	if err != nil {
		s.falhaInterna(w, "aparelho", err)
		return
	}
	if ap == nil {
		ap = []armazenamento.Aparicao{}
	}
	escreverJSON(w, http.StatusOK, map[string]any{
		"dispositivo": s.montarDispositivo(d),
		"aparicoes":   ap,
		"resumo_7d":   resumoPresenca(ap, s.agora()),
	})
}

type pedidoDispositivo struct {
	Apelido   *string `json:"apelido"`
	Notas     *string `json:"notas"`
	Confiavel *bool   `json:"confiavel"`
	VistoNovo bool    `json:"visto_novo"`
}

// textoLimpo recusa caracteres de controle e passa do tamanho.
func textoLimpo(s *string, max int, campo string) error {
	if s == nil {
		return nil
	}
	*s = strings.TrimSpace(*s)
	if utf8.RuneCountInString(*s) > max {
		return errors.New(campo + ": no máximo " + strconv.Itoa(max) + " caracteres")
	}
	for _, c := range *s {
		if unicode.IsControl(c) && c != '\n' {
			return errors.New(campo + ": caractere inválido")
		}
	}
	return nil
}

func (s *servidor) atualizarDispositivo(w http.ResponseWriter, r *http.Request) {
	id, ok := idDe(w, r)
	if !ok {
		return
	}
	var p pedidoDispositivo
	if !lerJSON(w, r, &p) {
		return
	}
	if err := errors.Join(textoLimpo(p.Apelido, 60, "apelido"), textoLimpo(p.Notas, 500, "notas")); err != nil {
		erroJSON(w, http.StatusBadRequest, err.Error())
		return
	}
	if p.Apelido != nil && strings.Contains(*p.Apelido, "\n") {
		erroJSON(w, http.StatusBadRequest, "apelido: uma linha só")
		return
	}
	d, err := s.Dispositivos.AtualizarDispositivo(r.Context(), id, armazenamento.Mudanca{
		Apelido: p.Apelido, Notas: p.Notas, Confiavel: p.Confiavel, VistoNovo: p.VistoNovo,
	})
	if errors.Is(err, armazenamento.ErrNaoEncontrado) {
		erroJSON(w, http.StatusNotFound, "aparelho não encontrado")
		return
	}
	if err != nil {
		s.falhaInterna(w, "aparelho", err)
		return
	}
	u := usuarioDe(r)
	detalhe := map[string]any{"mac": d.MAC}
	if p.Apelido != nil {
		detalhe["apelido"] = *p.Apelido
	}
	if p.Confiavel != nil {
		detalhe["confiavel"] = *p.Confiavel
	}
	if p.VistoNovo {
		detalhe["visto_novo"] = true
	}
	s.auditar(r, &u, "aparelho_alterado", strconv.FormatInt(id, 10), detalhe)
	escreverJSON(w, http.StatusOK, map[string]any{"dispositivo": s.montarDispositivo(d)})
}

func (s *servidor) varrer(w http.ResponseWriter, r *http.Request) {
	err := s.Descoberta.VarrerAgora(r.Context())
	switch {
	case errors.Is(err, dispositivos.ErrVarrendo):
		erroJSON(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, dispositivos.ErrVarreduraCedo):
		w.Header().Set("Retry-After", "60")
		erroJSON(w, http.StatusTooManyRequests, err.Error())
		return
	case err != nil:
		s.falhaInterna(w, "varredura", err)
		return
	}
	u := usuarioDe(r)
	s.auditar(r, &u, "varredura_manual", "", nil)
	escreverJSON(w, http.StatusAccepted, map[string]any{"estado": s.Descoberta.Estado()})
}

func (s *servidor) aparelhosNovos(w http.ResponseWriter, r *http.Request) {
	_, p, ok := lerPeriodo(w, r, "30d")
	if !ok {
		return
	}
	agora := s.agora()
	ds, err := s.Dispositivos.AparelhosNovos(r.Context(), agora.Add(-p.duracao), agora)
	if err != nil {
		s.falhaInterna(w, "aparelhos novos", err)
		return
	}
	lista := make([]respDispositivo, 0, len(ds))
	for _, d := range ds {
		lista = append(lista, s.montarDispositivo(d))
	}
	escreverJSON(w, http.StatusOK, map[string]any{"dispositivos": lista})
}

type respMovimento struct {
	armazenamento.Movimento
	Nome string `json:"nome"`
}

// movimentos: quem entrou e quem saiu, com o nome atual de cada aparelho.
func (s *servidor) movimentos(w http.ResponseWriter, r *http.Request) {
	_, p, ok := lerPeriodo(w, r, "24h")
	if !ok {
		return
	}
	agora := s.agora()
	mov, err := s.Dispositivos.Movimentos(r.Context(), agora.Add(-p.duracao), agora, dispositivos.Janela, 200)
	if err != nil {
		s.falhaInterna(w, "movimentos", err)
		return
	}
	lista, _, err := s.listaDispositivos(r.Context())
	if err != nil {
		s.falhaInterna(w, "movimentos", err)
		return
	}
	nomes := map[int64]string{}
	for _, d := range lista {
		nomes[d.ID] = d.Nome
	}
	resp := make([]respMovimento, 0, len(mov))
	for _, m := range mov {
		resp = append(resp, respMovimento{Movimento: m, Nome: nomes[m.DispositivoID]})
	}
	escreverJSON(w, http.StatusOK, map[string]any{"movimentos": resp, "janela_min": int(dispositivos.Janela.Minutes())})
}

type respResumoPresenca struct {
	Entradas    int     `json:"entradas"`
	Segundos    float64 `json:"segundos"`
	MediaS      float64 `json:"media_s"`      // tempo médio por vez
	HoraChegada *int    `json:"hora_chegada"` // hora em que costuma chegar (com 3+ entradas)
}

// resumoPresenca resume os últimos 7 dias a partir das aparições da ficha.
func resumoPresenca(ap []armazenamento.Aparicao, agora time.Time) respResumoPresenca {
	de := agora.Add(-7 * 24 * time.Hour)
	var r respResumoPresenca
	var horas []int
	for _, a := range ap {
		if a.Fim.Before(de) {
			continue
		}
		ini := a.Inicio
		if ini.Before(de) {
			ini = de
		} else {
			r.Entradas++
			horas = append(horas, a.Inicio.In(time.Local).Hour())
		}
		r.Segundos += a.Fim.Sub(ini).Seconds()
	}
	if r.Entradas > 0 {
		r.MediaS = r.Segundos / float64(r.Entradas)
	}
	if len(horas) >= 3 {
		slices.Sort(horas)
		h := horas[len(horas)/2]
		r.HoraChegada = &h
	}
	return r
}
