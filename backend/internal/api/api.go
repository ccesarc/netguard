// Package api expõe a API REST em /api/v1 e serve o frontend.
package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/netip"
	"time"

	"netguard/internal/armazenamento"
	"netguard/internal/auth"
	"netguard/internal/monitor"
)

const (
	nomeCookie   = "netguard_sessao"
	cabecalhoAPI = "X-NetGuard" // exigido em toda escrita: um site de fora não consegue mandar
	limiteCorpo  = 64 << 10
)

var Versao = "dev"

type Auditor interface {
	Auditar(ctx context.Context, a armazenamento.Auditoria) error
}

type FonteEstado interface {
	Estado() (monitor.Estado, bool)
}

type Dependencias struct {
	Auth           *auth.Servico
	Auditor        Auditor
	Monitor        FonteEstado
	Consultas      Consultas
	Consultas2B    ConsultasFase2B
	Ajustes        GerenteAjustes
	Velocidade     Velocimetro
	Wifi           FonteWifi
	Vizinhanca     VigiaVizinhanca
	ConsultasSaude ConsultasSaude
	Computador     FonteComputador
	Dispositivos   ConsultasDispositivos
	Descoberta     Descoberta
	Agora          func() time.Time // nil = time.Now
	SaudeBanco     func(ctx context.Context) error
	Redes          []netip.Prefix
	CookieSeguro   bool
	Log            *slog.Logger
	Web            http.Handler
}

type servidor struct {
	Dependencias
}

func (s *servidor) agora() time.Time {
	if s.Agora != nil {
		return s.Agora()
	}
	return time.Now()
}

func NovoServidor(d Dependencias) http.Handler {
	s := &servidor{d}
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/v1/saude", s.saude)
	mux.HandleFunc("POST /api/v1/auth/entrar", s.entrar)
	mux.Handle("POST /api/v1/auth/sair", s.exigirSessao(s.sair))
	mux.Handle("GET /api/v1/auth/eu", s.exigirSessao(s.eu))
	mux.Handle("GET /api/v1/visao-geral", s.exigirSessao(s.visaoGeral))
	mux.Handle("GET /api/v1/internet/historico", s.exigirSessao(s.historico))
	mux.Handle("GET /api/v1/internet/disponibilidade", s.exigirSessao(s.disponibilidade))
	mux.Handle("GET /api/v1/quedas", s.exigirSessao(s.listarQuedas))
	mux.Handle("GET /api/v1/velocidade", s.exigirSessao(s.velocidade))
	mux.Handle("POST /api/v1/velocidade/testar", s.exigirSessao(s.testarVelocidade))
	mux.Handle("GET /api/v1/wifi", s.exigirSessao(s.wifi))
	mux.Handle("GET /api/v1/wifi/vizinhanca", s.exigirSessao(s.vizinhanca))
	mux.Handle("POST /api/v1/wifi/vizinhanca/buscar", s.exigirSessao(s.buscarVizinhanca))
	mux.Handle("GET /api/v1/configuracao", s.exigirSessao(s.lerConfiguracao))
	mux.Handle("PUT /api/v1/configuracao", s.exigirSessao(s.salvarConfiguracao))
	mux.Handle("GET /api/v1/dispositivos", s.exigirSessao(s.listarDispositivos))
	mux.Handle("GET /api/v1/dispositivos/movimentos", s.exigirSessao(s.movimentos))
	mux.Handle("GET /api/v1/dispositivos/{id}", s.exigirSessao(s.detalheDispositivo))
	mux.Handle("PATCH /api/v1/dispositivos/{id}", s.exigirSessao(s.atualizarDispositivo))
	mux.Handle("POST /api/v1/dispositivos/varrer", s.exigirSessao(s.varrer))
	mux.Handle("GET /api/v1/aparelhos-novos", s.exigirSessao(s.aparelhosNovos))
	mux.Handle("GET /api/v1/saude-da-rede", s.exigirSessao(s.saudeDaRede))
	mux.Handle("GET /api/v1/computador", s.exigirSessao(s.computador))
	mux.HandleFunc("/api/", func(w http.ResponseWriter, r *http.Request) {
		erroJSON(w, http.StatusNotFound, "rota não encontrada")
	})
	mux.Handle("/", d.Web)

	var h http.Handler = mux
	h = s.protegerEscrita(h)
	h = s.redePermitida(h)
	h = cabecalhosSeguranca(h)
	h = s.registrar(h)
	h = s.recuperar(h)
	return h
}

// ---- respostas ----

func escreverJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func erroJSON(w http.ResponseWriter, status int, msg string) {
	escreverJSON(w, status, map[string]string{"erro": msg})
}

// lerJSON aceita só um objeto, sem campos desconhecidos, até limiteCorpo.
func lerJSON(w http.ResponseWriter, r *http.Request, destino any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limiteCorpo)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(destino); err != nil {
		var grande *http.MaxBytesError
		if errors.As(err, &grande) {
			erroJSON(w, http.StatusRequestEntityTooLarge, "corpo grande demais")
		} else {
			erroJSON(w, http.StatusBadRequest, "JSON inválido")
		}
		return false
	}
	if _, err := dec.Token(); err != io.EOF {
		erroJSON(w, http.StatusBadRequest, "JSON inválido: conteúdo depois do objeto")
		return false
	}
	return true
}

// ---- rotas ----

func (s *servidor) saude(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	resp := map[string]any{"status": "ok", "versao": Versao, "banco": "ok"}
	status := http.StatusOK
	if err := s.SaudeBanco(ctx); err != nil {
		resp["status"], resp["banco"] = "degradado", "indisponível"
		status = http.StatusServiceUnavailable
	}
	if e, ok := s.Monitor.Estado(); ok {
		resp["ultima_medicao"] = e.MedidoEm
	} else {
		resp["ultima_medicao"] = nil
	}
	escreverJSON(w, status, resp)
}

type pedidoEntrar struct {
	Nome  string `json:"nome"`
	Senha string `json:"senha"`
}

func (s *servidor) entrar(w http.ResponseWriter, r *http.Request) {
	var p pedidoEntrar
	if !lerJSON(w, r, &p) {
		return
	}
	if len(p.Nome) > 64 || len(p.Senha) > 512 || p.Nome == "" || p.Senha == "" {
		erroJSON(w, http.StatusBadRequest, "informe usuário e senha")
		return
	}
	ip := ipDe(r).String()
	u, token, expira, err := s.Auth.Entrar(r.Context(), p.Nome, p.Senha, ip, r.UserAgent())
	switch {
	case errors.Is(err, auth.ErrBloqueado):
		s.auditar(r, nil, "login_bloqueado", p.Nome, nil)
		w.Header().Set("Retry-After", "60")
		erroJSON(w, http.StatusTooManyRequests, err.Error())
		return
	case errors.Is(err, auth.ErrCredenciais):
		s.auditar(r, nil, "login_falhou", p.Nome, nil)
		erroJSON(w, http.StatusUnauthorized, err.Error())
		return
	case err != nil:
		s.Log.Error("login", "erro", err)
		erroJSON(w, http.StatusInternalServerError, "erro interno")
		return
	}
	s.gravarCookie(w, token, expira)
	s.auditar(r, &u, "login", "", nil)
	escreverJSON(w, http.StatusOK, map[string]any{"usuario": u})
}

func (s *servidor) sair(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(nomeCookie); err == nil {
		if err := s.Auth.Sair(r.Context(), c.Value); err != nil {
			s.Log.Warn("sair", "erro", err)
		}
	}
	u := usuarioDe(r)
	s.auditar(r, &u, "logout", "", nil)
	http.SetCookie(w, &http.Cookie{Name: nomeCookie, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.CookieSeguro})
	w.WriteHeader(http.StatusNoContent)
}

func (s *servidor) eu(w http.ResponseWriter, r *http.Request) {
	escreverJSON(w, http.StatusOK, map[string]any{"usuario": usuarioDe(r)})
}

func (s *servidor) gravarCookie(w http.ResponseWriter, token string, expira time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     nomeCookie,
		Value:    token,
		Path:     "/",
		Expires:  expira,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   s.CookieSeguro,
	})
}

func (s *servidor) auditar(r *http.Request, u *auth.Usuario, acao, alvo string, detalhe map[string]any) {
	a := armazenamento.Auditoria{Acao: acao, Alvo: alvo, Detalhe: detalhe, IP: ipDe(r).String()}
	if u != nil && u.ID != 0 {
		id := u.ID
		a.UsuarioID, a.Usuario = &id, u.Nome
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 3*time.Second)
	defer cancel()
	if err := s.Auditor.Auditar(ctx, a); err != nil {
		s.Log.Error("auditoria não gravada", "acao", acao, "erro", err)
	}
}
