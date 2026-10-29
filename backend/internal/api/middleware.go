package api

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"

	"netguard/internal/auth"
)

type chaveContexto int

const chaveUsuario chaveContexto = 0

func usuarioDe(r *http.Request) auth.Usuario {
	u, _ := r.Context().Value(chaveUsuario).(auth.Usuario)
	return u
}

// ipDe usa só o endereço da conexão. Não há proxy na frente, então
// X-Forwarded-For seria mentira fácil de forjar e é ignorado.
func ipDe(r *http.Request) netip.Addr {
	ap, err := netip.ParseAddrPort(r.RemoteAddr)
	if err != nil {
		host, _, _ := net.SplitHostPort(r.RemoteAddr)
		a, _ := netip.ParseAddr(host)
		return a.Unmap()
	}
	return ap.Addr().Unmap()
}

func (s *servidor) exigirSessao(proximo http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(nomeCookie)
		if err != nil {
			erroJSON(w, http.StatusUnauthorized, "faça login")
			return
		}
		u, novaExpira, err := s.Auth.Validar(r.Context(), c.Value)
		if errors.Is(err, auth.ErrSessaoInvalida) {
			erroJSON(w, http.StatusUnauthorized, "sessão expirada; faça login de novo")
			return
		}
		if err != nil {
			s.Log.Error("validar sessão", "erro", err)
			erroJSON(w, http.StatusServiceUnavailable, "não deu para conferir a sessão agora")
			return
		}
		if !novaExpira.IsZero() {
			s.gravarCookie(w, c.Value, novaExpira)
		}
		proximo(w, r.WithContext(context.WithValue(r.Context(), chaveUsuario, u)))
	})
}

// redePermitida recusa quem não está na rede local (ou nas redes configuradas).
func (s *servidor) redePermitida(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := ipDe(r)
		for _, p := range s.Redes {
			if ip.IsValid() && p.Contains(ip) {
				proximo.ServeHTTP(w, r)
				return
			}
		}
		s.Log.Warn("acesso de fora da rede local recusado", "ip", ip.String(), "caminho", r.URL.Path)
		http.Error(w, "acesso só pela rede local", http.StatusForbidden)
	})
}

// protegerEscrita barra pedidos de escrita vindos de outro site (CSRF):
// exige o cabeçalho X-NetGuard (navegador não manda de outra origem sem
// permissão) e confere Origin e Sec-Fetch-Site quando vierem.
func (s *servidor) protegerEscrita(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			proximo.ServeHTTP(w, r)
			return
		}
		if r.Header.Get(cabecalhoAPI) != "1" {
			erroJSON(w, http.StatusForbidden, "pedido recusado (falta cabeçalho)")
			return
		}
		if sfs := r.Header.Get("Sec-Fetch-Site"); sfs != "" && sfs != "same-origin" && sfs != "none" {
			erroJSON(w, http.StatusForbidden, "pedido de outra origem recusado")
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, err := url.Parse(o)
			if err != nil || !strings.EqualFold(u.Host, r.Host) {
				erroJSON(w, http.StatusForbidden, "pedido de outra origem recusado")
				return
			}
		}
		proximo.ServeHTTP(w, r)
	})
}

func cabecalhosSeguranca(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; "+
			"img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		proximo.ServeHTTP(w, r)
	})
}

type gravadorStatus struct {
	http.ResponseWriter
	status int
}

func (g *gravadorStatus) WriteHeader(s int) {
	g.status = s
	g.ResponseWriter.WriteHeader(s)
}

func (s *servidor) registrar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		inicio := time.Now()
		g := &gravadorStatus{ResponseWriter: w, status: http.StatusOK}
		proximo.ServeHTTP(g, r)
		nivel := s.Log.Debug
		if strings.HasPrefix(r.URL.Path, "/api/") && r.URL.Path != "/api/v1/saude" {
			nivel = s.Log.Info
		}
		nivel("http", "metodo", r.Method, "caminho", r.URL.Path, "status", g.status,
			"ms", time.Since(inicio).Milliseconds(), "ip", ipDe(r).String())
	})
}

func (s *servidor) recuperar(proximo http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if v := recover(); v != nil {
				if v == http.ErrAbortHandler {
					panic(v)
				}
				s.Log.Error("pânico na requisição", "valor", v, "caminho", r.URL.Path)
				erroJSON(w, http.StatusInternalServerError, "erro interno")
			}
		}()
		proximo.ServeHTTP(w, r)
	})
}
