package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"netguard/internal/ajustes"
	"netguard/internal/armazenamento"
	"netguard/internal/auth"
	"netguard/internal/computador"
	"netguard/internal/config"
	"netguard/internal/diagnostico"
	"netguard/internal/dispositivos"
	"netguard/internal/incidentes"
	"netguard/internal/monitor"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

type auditorFalso struct {
	mu    sync.Mutex
	acoes []string
}

func (a *auditorFalso) Auditar(_ context.Context, x armazenamento.Auditoria) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.acoes = append(a.acoes, x.Acao)
	return nil
}

type monitorFalso struct {
	e  monitor.Estado
	ok bool
}

func (m monitorFalso) Estado() (monitor.Estado, bool) { return m.e, m.ok }

type ambiente struct {
	h          http.Handler
	auditor    *auditorFalso
	banco      error
	ajustes    *ajustes.Gerente
	velocidade *velocimetroFalso
	disp       *dispositivosFalsos
	descoberta *descobertaFalsa
	vizinhanca *vizinhancaFalsa
}

const senha = "senha-bem-comprida"

func novoAmbiente(t *testing.T, mon monitorFalso) *ambiente {
	t.Helper()
	svc := auth.NovoServico(auth.NovoArmazemMemoria(), time.Hour)
	if err := svc.CriarUsuario(context.Background(), "dono", senha); err != nil {
		t.Fatal(err)
	}
	var redes []netip.Prefix
	for _, r := range config.RedesPrivadas {
		redes = append(redes, netip.MustParsePrefix(r))
	}
	amb := &ambiente{auditor: &auditorFalso{}, velocidade: &velocimetroFalso{}, disp: novosDispositivosFalsos(), descoberta: &descobertaFalsa{}, vizinhanca: &vizinhancaFalsa{}}
	amb.ajustes, _, _ = ajustes.NovoGerente(context.Background(), &armazemAjustes{}, ajustes.Padrao(10*time.Second))
	amb.h = NovoServidor(Dependencias{
		Auth:           svc,
		Auditor:        amb.auditor,
		Monitor:        mon,
		Consultas:      consultasFalsas{},
		Consultas2B:    consultasFalsas{},
		Ajustes:        amb.ajustes,
		Velocidade:     amb.velocidade,
		Wifi:           wifiFalso{},
		Vizinhanca:     amb.vizinhanca,
		ConsultasSaude: consultasFalsas{},
		Computador:     computadorFalso{},
		Dispositivos:   amb.disp,
		Descoberta:     amb.descoberta,
		Agora:          func() time.Time { return agoraTeste },
		SaudeBanco:     func(context.Context) error { return amb.banco },
		Redes:          redes,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
		Web:            http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { io.WriteString(w, "spa") }),
	})
	return amb
}

func (a *ambiente) pedir(metodo, caminho, corpo string, mod func(*http.Request)) *httptest.ResponseRecorder {
	var r *http.Request
	if corpo != "" {
		r = httptest.NewRequest(metodo, caminho, strings.NewReader(corpo))
	} else {
		r = httptest.NewRequest(metodo, caminho, nil)
	}
	r.RemoteAddr = "10.0.0.2:5555"
	r.Host = "10.0.0.209:8095"
	if metodo != http.MethodGet {
		r.Header.Set(cabecalhoAPI, "1")
		r.Header.Set("Content-Type", "application/json")
	}
	if mod != nil {
		mod(r)
	}
	w := httptest.NewRecorder()
	a.h.ServeHTTP(w, r)
	return w
}

func (a *ambiente) entrar(t *testing.T) *http.Cookie {
	t.Helper()
	w := a.pedir("POST", "/api/v1/auth/entrar", `{"nome":"dono","senha":"`+senha+`"}`, nil)
	if w.Code != 200 {
		t.Fatalf("login: %d %s", w.Code, w.Body)
	}
	for _, c := range w.Result().Cookies() {
		if c.Name == nomeCookie {
			if !c.HttpOnly || c.SameSite != http.SameSiteStrictMode {
				t.Fatalf("cookie sem proteção: %+v", c)
			}
			return c
		}
	}
	t.Fatal("login sem cookie")
	return nil
}

func comCookie(c *http.Cookie) func(*http.Request) {
	return func(r *http.Request) { r.AddCookie(c) }
}

func TestSaudeSemLogin(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	w := a.pedir("GET", "/api/v1/saude", "", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"banco":"ok"`) {
		t.Fatalf("saúde: %d %s", w.Code, w.Body)
	}
	a.banco = errors.New("fora")
	if w := a.pedir("GET", "/api/v1/saude", "", nil); w.Code != 503 {
		t.Fatalf("banco fora deveria dar 503: %d", w.Code)
	}
}

func TestRotasExigemLogin(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	for _, c := range []string{"/api/v1/visao-geral", "/api/v1/auth/eu"} {
		if w := a.pedir("GET", c, "", nil); w.Code != 401 {
			t.Errorf("%s sem login: %d", c, w.Code)
		}
	}
	bad := &http.Cookie{Name: nomeCookie, Value: strings.Repeat("x", 43)}
	if w := a.pedir("GET", "/api/v1/visao-geral", "", comCookie(bad)); w.Code != 401 {
		t.Errorf("cookie falso: %d", w.Code)
	}
}

func TestLoginFluxoCompleto(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)

	w := a.pedir("GET", "/api/v1/auth/eu", "", comCookie(c))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"nome":"dono"`) {
		t.Fatalf("eu: %d %s", w.Code, w.Body)
	}
	if w := a.pedir("POST", "/api/v1/auth/sair", "", comCookie(c)); w.Code != 204 {
		t.Fatalf("sair: %d", w.Code)
	}
	if w := a.pedir("GET", "/api/v1/auth/eu", "", comCookie(c)); w.Code != 401 {
		t.Fatalf("depois de sair: %d", w.Code)
	}
	got := strings.Join(a.auditor.acoes, ",")
	if got != "login,logout" {
		t.Fatalf("auditoria: %s", got)
	}
}

func TestLoginErradoEBloqueio(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	for i := 0; i < 5; i++ {
		if w := a.pedir("POST", "/api/v1/auth/entrar", `{"nome":"dono","senha":"errada-errada"}`, nil); w.Code != 401 {
			t.Fatalf("senha errada: %d", w.Code)
		}
	}
	w := a.pedir("POST", "/api/v1/auth/entrar", `{"nome":"dono","senha":"`+senha+`"}`, nil)
	if w.Code != 429 || w.Header().Get("Retry-After") == "" {
		t.Fatalf("esperava 429: %d", w.Code)
	}
	if !strings.Contains(strings.Join(a.auditor.acoes, ","), "login_falhou") {
		t.Fatal("falhas não auditadas")
	}
}

func TestValidacaoDoCorpo(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	casos := map[string]struct {
		corpo  string
		status int
	}{
		"campo a mais":  {`{"nome":"dono","senha":"x","admin":true}`, 400},
		"não é JSON":    {`nome=dono`, 400},
		"dois objetos":  {`{"nome":"dono","senha":"x"}{}`, 400},
		"vazio":         {`{"nome":"","senha":""}`, 400},
		"grande demais": {`{"nome":"` + strings.Repeat("a", 70000) + `","senha":"x"}`, 413},
	}
	for nome, c := range casos {
		if w := a.pedir("POST", "/api/v1/auth/entrar", c.corpo, nil); w.Code != c.status {
			t.Errorf("%s: esperava %d, veio %d", nome, c.status, w.Code)
		}
	}
}

func TestProtecaoCSRF(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	corpo := `{"nome":"dono","senha":"` + senha + `"}`
	semCabecalho := func(r *http.Request) { r.Header.Del(cabecalhoAPI) }
	if w := a.pedir("POST", "/api/v1/auth/entrar", corpo, semCabecalho); w.Code != 403 {
		t.Errorf("sem cabeçalho: %d", w.Code)
	}
	outraOrigem := func(r *http.Request) { r.Header.Set("Origin", "http://site-mal.com") }
	if w := a.pedir("POST", "/api/v1/auth/entrar", corpo, outraOrigem); w.Code != 403 {
		t.Errorf("Origin de fora: %d", w.Code)
	}
	crossSite := func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }
	if w := a.pedir("POST", "/api/v1/auth/entrar", corpo, crossSite); w.Code != 403 {
		t.Errorf("cross-site: %d", w.Code)
	}
	mesmaOrigem := func(r *http.Request) {
		r.Header.Set("Origin", "http://10.0.0.209:8095")
		r.Header.Set("Sec-Fetch-Site", "same-origin")
	}
	if w := a.pedir("POST", "/api/v1/auth/entrar", corpo, mesmaOrigem); w.Code != 200 {
		t.Errorf("mesma origem deveria passar: %d %s", w.Code, w.Body)
	}
}

func TestSoRedeLocal(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	for _, ip := range []string{"8.8.8.8:1234", "[2001:db8::1]:1234"} {
		ipFora := func(r *http.Request) { r.RemoteAddr = ip }
		if w := a.pedir("GET", "/api/v1/saude", "", ipFora); w.Code != 403 {
			t.Errorf("%s deveria ser recusado: %d", ip, w.Code)
		}
	}
	local := func(r *http.Request) { r.RemoteAddr = "127.0.0.1:1" }
	if w := a.pedir("GET", "/", "", local); w.Code != 200 || w.Body.String() != "spa" {
		t.Errorf("localhost: %d", w.Code)
	}
}

func TestCabecalhosDeSeguranca(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	w := a.pedir("GET", "/api/v1/saude", "", nil)
	for _, h := range []string{"Content-Security-Policy", "X-Content-Type-Options", "X-Frame-Options", "Referrer-Policy"} {
		if w.Header().Get(h) == "" {
			t.Errorf("falta %s", h)
		}
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Error("API não pode ir para cache")
	}
	if w := a.pedir("GET", "/api/v1/nao-existe", "", nil); w.Code != 404 {
		t.Errorf("rota inexistente da API: %d", w.Code)
	}
}

func TestVisaoGeral(t *testing.T) {
	// Sem medição ainda: tudo indisponível, nada de zero.
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	var v map[string]map[string]any
	w := a.pedir("GET", "/api/v1/visao-geral", "", comCookie(c))
	json.Unmarshal(w.Body.Bytes(), &v)
	if v["gateway"]["disponivel"] != false || v["gateway"]["motivo"] == "" {
		t.Fatalf("sem medição: %s", w.Body)
	}
	if _, tem := v["gateway"]["latencia_media_ms"]; tem {
		t.Fatal("não pode mandar latência sem medição")
	}
	if v["internet"]["disponivel"] != false {
		t.Fatalf("internet sem medição: %s", w.Body)
	}

	// Com medição.
	e := monitor.Estado{
		MedidoEm:  time.Now(),
		Interface: monitor.EstadoInterface{Nome: "wlan0", Tipo: "wifi", Ativa: true, IPv4: "10.0.0.209/24", Gateway: netip.MustParseAddr("10.0.0.1")},
		Gateway:   &monitor.MedidaPing{Alvo: "10.0.0.1", OK: true, Enviados: 5, Recebidos: 5, MediaMS: 6.8},
	}
	a = novoAmbiente(t, monitorFalso{e: e, ok: true})
	c = a.entrar(t)
	w = a.pedir("GET", "/api/v1/visao-geral", "", comCookie(c))
	v = nil
	json.Unmarshal(w.Body.Bytes(), &v)
	if v["gateway"]["disponivel"] != true || v["gateway"]["latencia_media_ms"] != 6.8 {
		t.Fatalf("gateway: %s", w.Body)
	}
	if v["interface"]["gateway"] != "10.0.0.1" || v["interface"]["ok"] != true {
		t.Fatalf("interface: %s", w.Body)
	}
	if v["alertas"]["disponivel"] != false || v["alertas"]["motivo"] == "" {
		t.Fatalf("módulos futuros deveriam vir indisponíveis: %s", w.Body)
	}
}

var agoraTeste = time.Date(2026, 10, 29, 12, 0, 30, 0, time.UTC)

type consultasFalsas struct{}

func (consultasFalsas) Historico(_ context.Context, de, ate time.Time, passo time.Duration) ([]armazenamento.PontoHistorico, error) {
	// só o penúltimo ponto tem dado; o resto é buraco
	gw, fora := 7.04, 0.0
	return []armazenamento.PontoHistorico{{Em: ate.Truncate(passo).Add(-passo), GatewayMS: &gw, FracaoFora: &fora}}, nil
}

func (consultasFalsas) Disponibilidade(_ context.Context, de, ate time.Time) (armazenamento.Disponibilidade, error) {
	if ate.Sub(de) > 48*time.Hour {
		return armazenamento.Disponibilidade{}, nil // sem dados no período longo
	}
	return armazenamento.Disponibilidade{CiclosNoAr: 99, CiclosTotal: 100, SegundosCobertos: 43200, Quedas: 1, SegundosFora: 40}, nil
}

func (consultasFalsas) Incidentes(context.Context, time.Time, time.Time, int) ([]incidentes.Incidente, error) {
	fim := agoraTeste.Add(-time.Hour)
	return []incidentes.Incidente{
		{ID: 2, Inicio: agoraTeste.Add(-time.Minute), Causa: diagnostico.CausaExterno, Causas: []diagnostico.Causa{diagnostico.CausaExterno}},
		{ID: 1, Inicio: fim.Add(-40 * time.Second), Fim: &fim, Causa: diagnostico.CausaRedeLocal, Causas: []diagnostico.Causa{diagnostico.CausaRedeLocal}},
	}, nil
}

func TestHistorico(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/internet/historico?periodo=1h", "", comCookie(c))
	if w.Code != 200 {
		t.Fatalf("histórico: %d %s", w.Code, w.Body)
	}
	var h respHistorico
	json.Unmarshal(w.Body.Bytes(), &h)
	if h.PassoS != 60 || len(h.T) < 60 || len(h.T) != len(h.GatewayMS) || len(h.T) != len(h.Fora) {
		t.Fatalf("formato: passo=%d pontos=%d/%d", h.PassoS, len(h.T), len(h.GatewayMS))
	}
	n := len(h.T)
	if h.GatewayMS[n-2] == nil || *h.GatewayMS[n-2] != 7 || h.GatewayMS[0] != nil {
		t.Fatalf("pontos: penúltimo=%v primeiro=%v", h.GatewayMS[n-2], h.GatewayMS[0])
	}
	if w := a.pedir("GET", "/api/v1/internet/historico?periodo=1ano", "", comCookie(c)); w.Code != 400 {
		t.Fatalf("período inválido: %d", w.Code)
	}
	if w := a.pedir("GET", "/api/v1/internet/historico", "", nil); w.Code != 401 {
		t.Fatalf("sem login: %d", w.Code)
	}
}

func TestDisponibilidade(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/internet/disponibilidade", "", comCookie(c))
	var d map[string]respDisponibilidade
	json.Unmarshal(w.Body.Bytes(), &d)
	h := d["24h"]
	if !h.Disponivel || h.Percentual != 0.99 || h.Cobertura != 0.5 || h.Quedas != 1 {
		t.Fatalf("24h: %+v", h)
	}
	if d["7d"].Disponivel || d["7d"].Motivo == "" || d["7d"].Percentual != 0 {
		t.Fatalf("sem dados não pode virar 0%% nem 100%%: %+v", d["7d"])
	}
}

func TestListarQuedas(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/quedas", "", comCookie(c))
	var r struct{ Quedas []respQueda }
	json.Unmarshal(w.Body.Bytes(), &r)
	if len(r.Quedas) != 2 {
		t.Fatalf("quedas: %s", w.Body)
	}
	aberta, fechada := r.Quedas[0], r.Quedas[1]
	if !aberta.EmAndamento || aberta.DuracaoS != 60 || aberta.Causa.Texto == "" {
		t.Fatalf("aberta: %+v", aberta)
	}
	if fechada.EmAndamento || fechada.DuracaoS != 40 || fechada.Causa.Causa != diagnostico.CausaRedeLocal {
		t.Fatalf("fechada: %+v", fechada)
	}
	for _, q := range []string{"limite=0", "limite=9999", "limite=abc", "periodo=x"} {
		if w := a.pedir("GET", "/api/v1/quedas?"+q, "", comCookie(c)); w.Code != 400 {
			t.Errorf("%s: %d", q, w.Code)
		}
	}
}

func TestVisaoGeralComDiagnostico(t *testing.T) {
	e := monitor.Estado{
		MedidoEm:  agoraTeste,
		Interface: monitor.EstadoInterface{Nome: "wlan0", Ativa: true, IPv4: "10.0.0.209/24", Gateway: netip.MustParseAddr("10.0.0.1")},
		Gateway:   &monitor.MedidaPing{Alvo: "10.0.0.1", OK: true, MediaMS: 7},
		Externos: []monitor.MedidaPing{
			{Alvo: "1.1.1.1", OK: true, MediaMS: 20},
			{Alvo: "8.8.8.8", OK: false, Perda: 1},
		},
		DNSSistema:  []monitor.Medida{{Alvo: "8.8.8.8", OK: true, MS: 30}},
		Diagnostico: diagnostico.Resultado{Estado: diagnostico.Fora, Causa: diagnostico.CausaExterno},
		Queda:       &incidentes.Incidente{ID: 3, Inicio: agoraTeste.Add(-30 * time.Second), Causa: diagnostico.CausaExterno},
	}
	a := novoAmbiente(t, monitorFalso{e: e, ok: true})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/visao-geral", "", comCookie(c))
	var v respVisaoGeral
	json.Unmarshal(w.Body.Bytes(), &v)
	if v.Internet.Estado != diagnostico.Fora || v.Internet.Causa == nil || v.Internet.Causa.Texto == "" {
		t.Fatalf("internet: %+v", v.Internet)
	}
	if *v.Internet.LatenciaMS != 20 || *v.Internet.Perda != 0.5 {
		t.Fatalf("média dos externos: %v %v", *v.Internet.LatenciaMS, *v.Internet.Perda)
	}
	if !v.DNS.Disponivel || !v.DNS.OK {
		t.Fatalf("dns: %+v", v.DNS)
	}
	if v.Queda == nil || v.Queda.DuracaoS != 30 || !v.Queda.EmAndamento {
		t.Fatalf("queda: %+v", v.Queda)
	}
}

type armazemAjustes struct{ salvo *ajustes.Ajustes }

func (a *armazemAjustes) LerAjustes(context.Context) (ajustes.Ajustes, bool, error) {
	if a.salvo == nil {
		return ajustes.Ajustes{}, false, nil
	}
	return *a.salvo, true, nil
}

func (a *armazemAjustes) SalvarAjustes(_ context.Context, x ajustes.Ajustes, _ string) error {
	a.salvo = &x
	return nil
}

type velocimetroFalso struct {
	err     error
	pedidos int
}

func (v *velocimetroFalso) Estado() velocidade.Estado {
	d := 242.0
	return velocidade.Estado{Ultimo: &velocidade.Resultado{Em: agoraTeste, Origem: "agendado", DownloadMbps: &d}}
}

func (v *velocimetroFalso) TestarAgora() error { v.pedidos++; return v.err }

type wifiFalso struct{}

func (wifiFalso) Ultima() (*wifi.Amostra, string) {
	return &wifi.Amostra{SSID: "MINHA_REDE_5G", Sinal: 79, Canal: 48, TaxaMbps: 1170}, ""
}

func (consultasFalsas) TestesVelocidade(context.Context, time.Time, time.Time) ([]velocidade.Resultado, error) {
	d, u := 242.0, 85.0
	return []velocidade.Resultado{{Em: agoraTeste.Add(-time.Hour), Origem: "agendado", DownloadMbps: &d, UploadMbps: &u}}, nil
}

func (consultasFalsas) BytesTesteVelocidade(context.Context, time.Time, time.Time) (int64, error) {
	return 105 << 20, nil
}

func (consultasFalsas) HistoricoWifi(_ context.Context, de, ate time.Time, passo time.Duration) ([]armazenamento.PontoWifi, error) {
	s := 79.0
	return []armazenamento.PontoWifi{{Em: ate.Truncate(passo).Add(-passo), Sinal: &s, SinalMin: &s}}, nil
}

func TestVelocidade(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/velocidade", "", comCookie(c))
	var v map[string]any
	json.Unmarshal(w.Body.Bytes(), &v)
	if w.Code != 200 || v["gasto_30d_mb"] != 105.0 || v["estimativa_mes_mb"] != 3150.0 || len(v["testes"].([]any)) != 1 {
		t.Fatalf("velocidade: %d %s", w.Code, w.Body)
	}

	if w := a.pedir("POST", "/api/v1/velocidade/testar", "", comCookie(c)); w.Code != 202 {
		t.Fatalf("testar: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(strings.Join(a.auditor.acoes, ","), "teste_velocidade") {
		t.Fatal("teste manual não auditado")
	}
	a.velocidade.err = velocidade.ErrRodando
	if w := a.pedir("POST", "/api/v1/velocidade/testar", "", comCookie(c)); w.Code != 409 {
		t.Fatalf("rodando: %d", w.Code)
	}
	a.velocidade.err = velocidade.ErrCedo
	if w := a.pedir("POST", "/api/v1/velocidade/testar", "", comCookie(c)); w.Code != 429 {
		t.Fatalf("cedo: %d", w.Code)
	}
	semCabecalho := func(r *http.Request) { r.Header.Del(cabecalhoAPI) }
	antes := a.velocidade.pedidos
	if w := a.pedir("POST", "/api/v1/velocidade/testar", "", func(r *http.Request) { comCookie(c)(r); semCabecalho(r) }); w.Code != 403 || a.velocidade.pedidos != antes {
		t.Fatalf("sem cabeçalho não pode disparar teste: %d", w.Code)
	}
}

func TestWifi(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/wifi?periodo=1h", "", comCookie(c))
	var v struct {
		Atual struct {
			Disponivel bool
			Amostra    wifi.Amostra
		}
		Historico respHistoricoWifi
	}
	json.Unmarshal(w.Body.Bytes(), &v)
	n := len(v.Historico.T)
	if !v.Atual.Disponivel || v.Atual.Amostra.Sinal != 79 || n < 60 || v.Historico.Sinal[n-2] == nil || v.Historico.Sinal[0] != nil {
		t.Fatalf("wifi: %s", w.Body)
	}
}

func TestConfiguracao(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/configuracao", "", comCookie(c))
	var resp struct {
		Ajustes ajustes.Ajustes
		Padrao  ajustes.Ajustes
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Ajustes.IntervaloS != 10 || resp.Padrao.Velocidade.VezesPorDia != 3 {
		t.Fatalf("ler: %s", w.Body)
	}

	novo := resp.Ajustes
	novo.IntervaloS = 30
	novo.URLs = []string{"https://exemplo.com.br"}
	corpo, _ := json.Marshal(novo)
	w = a.pedir("PUT", "/api/v1/configuracao", string(corpo), comCookie(c))
	if w.Code != 200 || a.ajustes.Atuais().IntervaloS != 30 {
		t.Fatalf("salvar: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(strings.Join(a.auditor.acoes, ","), "configuracao_alterada") {
		t.Fatal("mudança não auditada")
	}

	ruim := novo
	ruim.IntervaloS, ruim.Externos = 1, []string{"10.0.0.1"}
	corpo, _ = json.Marshal(ruim)
	w = a.pedir("PUT", "/api/v1/configuracao", string(corpo), comCookie(c))
	var e struct{ Erros []string }
	json.Unmarshal(w.Body.Bytes(), &e)
	if w.Code != 400 || len(e.Erros) != 2 || a.ajustes.Atuais().IntervaloS != 30 {
		t.Fatalf("inválido: %d %s", w.Code, w.Body)
	}
	if w := a.pedir("PUT", "/api/v1/configuracao", `{"intervalo_s":30,"comando":"rm -rf /"}`, comCookie(c)); w.Code != 400 {
		t.Fatalf("campo desconhecido: %d", w.Code)
	}
	if w := a.pedir("PUT", "/api/v1/configuracao", string(corpo), nil); w.Code != 401 {
		t.Fatalf("sem login: %d", w.Code)
	}
}

func TestDiferencas(t *testing.T) {
	a := ajustes.Padrao(10 * time.Second)
	b := a
	b.IntervaloS = 30
	d := diferencas(a, b)
	if len(d) != 1 || d["intervalo_s"] == nil {
		t.Fatalf("diferenças: %v", d)
	}
}

type dispositivosFalsos struct {
	lista   map[int64]armazenamento.Dispositivo
	mudanca armazenamento.Mudanca
}

func novosDispositivosFalsos() *dispositivosFalsos {
	return &dispositivosFalsos{lista: map[int64]armazenamento.Dispositivo{
		1: {ID: 1, MAC: "54:6c:ac:12:34:43", IP: "10.0.0.1", NomeDescoberto: "meuintelbras", Fabricante: "Intelbras", UltimoVisto: agoraTeste.Add(-time.Minute)},
		2: {ID: 2, MAC: "56:e2:c7:12:34:ac", IP: "10.0.0.2", MACAleatorio: true, UltimoVisto: agoraTeste.Add(-2 * time.Hour)},
		3: {ID: 3, MAC: "00:11:22:33:44:55", IP: "10.0.0.77", AlertarNovo: true, Fabricante: "Samsung", UltimoVisto: agoraTeste},
		4: {ID: 4, MAC: "14:bd:61:12:34:c4", IP: "10.0.0.209", EsteAparelho: true, UltimoVisto: agoraTeste},
	}}
}

func (d *dispositivosFalsos) Dispositivos(context.Context) ([]armazenamento.Dispositivo, error) {
	var r []armazenamento.Dispositivo
	for i := int64(1); i <= int64(len(d.lista)); i++ {
		r = append(r, d.lista[i])
	}
	return r, nil
}

func (d *dispositivosFalsos) Dispositivo(_ context.Context, id int64) (armazenamento.Dispositivo, error) {
	x, ok := d.lista[id]
	if !ok {
		return x, armazenamento.ErrNaoEncontrado
	}
	return x, nil
}

func (d *dispositivosFalsos) Aparicoes(context.Context, int64, int) ([]armazenamento.Aparicao, error) {
	return []armazenamento.Aparicao{{Inicio: agoraTeste.Add(-time.Hour), Fim: agoraTeste, IP: "10.0.0.77"}}, nil
}

func (d *dispositivosFalsos) AtualizarDispositivo(_ context.Context, id int64, m armazenamento.Mudanca) (armazenamento.Dispositivo, error) {
	x, ok := d.lista[id]
	if !ok {
		return x, armazenamento.ErrNaoEncontrado
	}
	d.mudanca = m
	if m.Apelido != nil {
		x.Apelido = *m.Apelido
	}
	if m.Confiavel != nil {
		x.Confiavel = *m.Confiavel
	}
	d.lista[id] = x
	return x, nil
}

func (d *dispositivosFalsos) AparelhosNovos(context.Context, time.Time, time.Time) ([]armazenamento.Dispositivo, error) {
	return []armazenamento.Dispositivo{d.lista[3]}, nil
}

type descobertaFalsa struct{ err error }

func (descobertaFalsa) Estado() dispositivos.Estado {
	return dispositivos.Estado{VarreduraAtiva: false}
}
func (d *descobertaFalsa) VarrerAgora(context.Context) error { return d.err }

func TestListarDispositivos(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/dispositivos", "", comCookie(c))
	var r struct {
		Resumo       resumoDispositivos
		Dispositivos []respDispositivo
	}
	json.Unmarshal(w.Body.Bytes(), &r)
	if r.Resumo.Total != 4 || r.Resumo.NaRede != 3 || r.Resumo.Novos != 1 {
		t.Fatalf("resumo: %+v", r.Resumo)
	}
	nomes := map[int64]string{}
	for _, d := range r.Dispositivos {
		nomes[d.ID] = d.Nome
	}
	if nomes[1] != "meuintelbras" || nomes[2] != "Aparelho sem nome (MAC aleatório)" || nomes[3] != "Aparelho Samsung" || nomes[4] != "Este aparelho (NetGuard)" {
		t.Fatalf("nomes: %v", nomes)
	}
	if r.Dispositivos[1].NaRede {
		t.Fatal("visto há 2 h não está na rede")
	}
}

func TestDetalheEAtualizarDispositivo(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/dispositivos/3", "", comCookie(c))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"aparicoes":[{`) {
		t.Fatalf("detalhe: %d %s", w.Code, w.Body)
	}
	for _, id := range []string{"0", "abc", "-1"} {
		if w := a.pedir("GET", "/api/v1/dispositivos/"+id, "", comCookie(c)); w.Code != 400 {
			t.Errorf("id %s: %d", id, w.Code)
		}
	}
	if w := a.pedir("GET", "/api/v1/dispositivos/999", "", comCookie(c)); w.Code != 404 {
		t.Fatalf("inexistente: %d", w.Code)
	}

	w = a.pedir("PATCH", "/api/v1/dispositivos/3", `{"apelido":"  Celular da Ana  ","confiavel":true}`, comCookie(c))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"nome":"Celular da Ana"`) {
		t.Fatalf("atualizar: %d %s", w.Code, w.Body)
	}
	if !strings.Contains(strings.Join(a.auditor.acoes, ","), "aparelho_alterado") {
		t.Fatal("mudança não auditada")
	}
	ruins := map[string]string{
		"apelido longo":  `{"apelido":"` + strings.Repeat("a", 61) + `"}`,
		"duas linhas":    `{"apelido":"a\nb"}`,
		"controle":       `{"apelido":"a\u0007b"}`,
		"campo estranho": `{"mac":"00:00:00:00:00:00"}`,
	}
	for nome, corpo := range ruins {
		if w := a.pedir("PATCH", "/api/v1/dispositivos/3", corpo, comCookie(c)); w.Code != 400 {
			t.Errorf("%s: %d %s", nome, w.Code, w.Body)
		}
	}
}

func TestVarrerAgora(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	if w := a.pedir("POST", "/api/v1/dispositivos/varrer", "", comCookie(c)); w.Code != 202 {
		t.Fatalf("varrer: %d", w.Code)
	}
	if !strings.Contains(strings.Join(a.auditor.acoes, ","), "varredura_manual") {
		t.Fatal("varredura não auditada")
	}
	a.descoberta.err = dispositivos.ErrVarreduraCedo
	if w := a.pedir("POST", "/api/v1/dispositivos/varrer", "", comCookie(c)); w.Code != 429 {
		t.Fatalf("cedo: %d", w.Code)
	}
	a.descoberta.err = dispositivos.ErrVarrendo
	if w := a.pedir("POST", "/api/v1/dispositivos/varrer", "", comCookie(c)); w.Code != 409 {
		t.Fatalf("rodando: %d", w.Code)
	}
}

func TestVisaoGeralComAparelhos(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/visao-geral", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"resumo":{"total":4,"na_rede":3,"novos":1}`) {
		t.Fatalf("visão geral: %s", w.Body)
	}
	w = a.pedir("GET", "/api/v1/aparelhos-novos", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"nome":"Aparelho Samsung"`) {
		t.Fatalf("aparelhos novos: %s", w.Body)
	}
}

func TestNomeCurto(t *testing.T) {
	casos := map[string]string{
		"Amazon Technologies Inc.":     "Amazon",
		"LG Electronics":               "LG",
		"Apple, Inc.":                  "Apple",
		"Intelbras":                    "Intelbras",
		"Xiaomi Communications Co Ltd": "Xiaomi",
		"Samsung Electronics Co.,Ltd":  "Samsung",
	}
	for de, para := range casos {
		if got := nomeCurto(de); got != para {
			t.Errorf("%q: esperava %q, veio %q", de, para, got)
		}
	}
}

type vizinhancaFalsa struct {
	viz *wifi.Vizinhanca
	err error
}

func (v *vizinhancaFalsa) Ultima() (*wifi.Vizinhanca, string, bool) {
	return v.viz, "Aguardando a primeira busca.", false
}
func (v *vizinhancaFalsa) BuscarAgora(context.Context) error { return v.err }
func (v *vizinhancaFalsa) BuscandoDesde() time.Time          { return time.Time{} }

func TestVizinhanca(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/wifi/vizinhanca", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"disponivel":false`) || !strings.Contains(w.Body.String(), "Aguardando") {
		t.Fatalf("antes da busca: %s", w.Body)
	}
	redes := []wifi.Rede{
		{SSID: "casa5", BSSID: "54:6C:AC:12:34:45", Canal: 48, FreqMHz: 5240, Sinal: 79, Conectada: true},
		{SSID: "viz", BSSID: "AA:00:00:00:00:01", Canal: 149, FreqMHz: 5745, Sinal: 72},
	}
	a.vizinhanca.viz = &wifi.Vizinhanca{Em: agoraTeste, Redes: redes, F24: wifi.Analisar24(redes), F5: wifi.Analisar5(redes)}
	w = a.pedir("GET", "/api/v1/wifi/vizinhanca", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"canal_atual":48`) || strings.Contains(w.Body.String(), "AA:00:00") {
		t.Fatalf("depois da busca (e sem expor BSSID): %s", w.Body)
	}
	if w := a.pedir("POST", "/api/v1/wifi/vizinhanca/buscar", "", comCookie(c)); w.Code != 202 {
		t.Fatalf("buscar: %d", w.Code)
	}
	a.vizinhanca.err = wifi.ErrBuscaCedo
	if w := a.pedir("POST", "/api/v1/wifi/vizinhanca/buscar", "", comCookie(c)); w.Code != 429 {
		t.Fatalf("cedo: %d", w.Code)
	}
}

func (consultasFalsas) ResumoRede(context.Context, time.Time, time.Time) (armazenamento.ResumoRede, error) {
	m, p := 48.0, 0.0
	return armazenamento.ResumoRede{LatenciaMediana: &m, PerdaMedia: &p, Amostras: 8000}, nil
}

type computadorFalso struct{}

func (computadorFalso) Resumo() (*computador.Resumo, bool) {
	t := 56.0
	return &computador.Resumo{Agora: computador.Leitura{MemTotalMB: 7808, MemDisponivelMB: 5524, Nucleos: 4, Carga5: 0.3,
		Discos: []computador.Disco{{Ponto: "/", TotalGB: 443, LivreGB: 280, UsoPct: 37}}, TempCPU: &t, TempSensor: "coretemp"}, TempMaxHora: &t, MemMinHoraMB: 5000}, true
}

func TestSaudeDaRede(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/saude-da-rede", "", comCookie(c))
	if w.Code != 200 {
		t.Fatalf("saúde: %d %s", w.Code, w.Body)
	}
	var r struct {
		Rotulo string
		Itens  []struct{ ID, Estado string }
	}
	json.Unmarshal(w.Body.Bytes(), &r)
	por := map[string]string{}
	for _, it := range r.Itens {
		por[it.ID] = it.Estado
	}
	if por["latencia"] != "ok" || por["memoria"] != "ok" || por["temperatura"] != "ok" || por["aparelhos"] != "atencao" {
		t.Fatalf("itens: %v", por)
	}
	if w := a.pedir("GET", "/api/v1/computador", "", comCookie(c)); !strings.Contains(w.Body.String(), `"temp_cpu":56`) {
		t.Fatalf("computador: %s", w.Body)
	}
	if w := a.pedir("GET", "/api/v1/saude-da-rede", "", nil); w.Code != 401 {
		t.Fatalf("sem login: %d", w.Code)
	}
}

func (d *dispositivosFalsos) Movimentos(context.Context, time.Time, time.Time, time.Duration, int) ([]armazenamento.Movimento, error) {
	return []armazenamento.Movimento{
		{Tipo: "entrou", Em: agoraTeste.Add(-time.Minute), DispositivoID: 3, IP: "10.0.0.77"},
		{Tipo: "saiu", Em: agoraTeste.Add(-2 * time.Hour), DispositivoID: 2, IP: "10.0.0.2"},
	}, nil
}

func TestMovimentos(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/dispositivos/movimentos", "", comCookie(c))
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"tipo":"entrou"`) || !strings.Contains(w.Body.String(), `"nome":"Aparelho Samsung"`) || !strings.Contains(w.Body.String(), `"janela_min":10`) {
		t.Fatalf("movimentos: %d %s", w.Code, w.Body)
	}
}

func (d *dispositivosFalsos) Presencas(context.Context, time.Time, time.Time, time.Duration) (map[int64]armazenamento.Presenca, error) {
	return map[int64]armazenamento.Presenca{1: {Entradas: 1, Segundos: 7 * 86400}, 3: {Entradas: 14, Segundos: 31 * 3600}}, nil
}

func TestResumoPresenca(t *testing.T) {
	agora := time.Date(2026, 10, 29, 20, 0, 0, 0, time.Local)
	dia := func(d, h int) time.Time {
		return agora.AddDate(0, 0, -d).Truncate(24 * time.Hour).Add(time.Duration(h) * time.Hour)
	}
	ap := []armazenamento.Aparicao{
		{Inicio: dia(1, 18).In(time.Local), Fim: dia(1, 20)},
		{Inicio: dia(2, 18).In(time.Local), Fim: dia(2, 19)},
		{Inicio: dia(3, 19).In(time.Local), Fim: dia(3, 22)},
		{Inicio: agora.AddDate(0, 0, -8), Fim: agora.AddDate(0, 0, -7).Add(time.Hour)}, // começou antes: só o pedaço conta
	}
	r := resumoPresenca(ap, agora)
	if r.Entradas != 3 || r.Segundos != 7*3600 || r.HoraChegada == nil {
		t.Fatalf("resumo: %+v", r)
	}
	if r.MediaS != 7*3600/3.0 {
		t.Fatalf("média: %v", r.MediaS)
	}
	if poucas := resumoPresenca(ap[:2], agora); poucas.HoraChegada != nil {
		t.Fatal("com menos de 3 entradas não diz a hora de chegada")
	}
}

func TestListaTrazPresenca(t *testing.T) {
	a := novoAmbiente(t, monitorFalso{})
	c := a.entrar(t)
	w := a.pedir("GET", "/api/v1/dispositivos", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"presenca_7d":{"entradas":14,"segundos":111600}`) {
		t.Fatalf("lista sem presença: %s", w.Body)
	}
	w = a.pedir("GET", "/api/v1/dispositivos/3", "", comCookie(c))
	if !strings.Contains(w.Body.String(), `"resumo_7d":{"entradas":1`) {
		t.Fatalf("ficha sem resumo: %s", w.Body)
	}
}
