package monitor

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"netguard/internal/ajustes"
	"netguard/internal/armazenamento"
	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
)

// Cópia do /proc/net/route deste PC (Wi-Fi + cabo desligado + docker).
const rotasExemplo = `Iface	Destination	Gateway 	Flags	RefCnt	Use	Metric	Mask		MTU	Window	IRTT
wlan0	00000000	0100000A	0003	0	0	600	00000000	0	0	0
wlan0	0000000A	00000000	0001	0	0	600	00FFFFFF	0	0	0
enp3s0f0	00000000	0101A8C0	0003	0	0	100	00000000	0	0	0
docker0	000011AC	00000000	0001	0	0	0	0000FFFF	0	0	0
`

func TestAnalisarRotas(t *testing.T) {
	rotas, err := analisarRotas(strings.NewReader(rotasExemplo))
	if err != nil {
		t.Fatal(err)
	}
	if len(rotas) != 2 {
		t.Fatalf("esperava 2 rotas padrão, veio %d: %+v", len(rotas), rotas)
	}
	r, ok := rotaPadrao(rotas, "")
	if !ok || r.iface != "enp3s0f0" || r.gateway != netip.MustParseAddr("192.168.1.1") {
		t.Fatalf("menor métrica errada: %+v", r)
	}
	r, ok = rotaPadrao(rotas, "wlan0")
	if !ok || r.gateway != netip.MustParseAddr("10.0.0.1") {
		t.Fatalf("rota do wlan0: %+v", r)
	}
	if _, ok := rotaPadrao(rotas, "eth9"); ok {
		t.Fatal("interface sem rota não deveria ter rota")
	}
	if _, err := analisarRotas(strings.NewReader("cab\nwlan0 ZZ 0 0 0 0 0 0\n")); err == nil {
		t.Fatal("linha inválida deveria dar erro")
	}
}

func raizFalsa(t *testing.T, rotas string, ifaces map[string]string) string {
	t.Helper()
	raiz := t.TempDir()
	os.MkdirAll(filepath.Join(raiz, "proc/net"), 0o755)
	os.WriteFile(filepath.Join(raiz, "proc/net/route"), []byte(rotas), 0o644)
	for nome, estado := range ifaces {
		d := filepath.Join(raiz, "sys/class/net", nome)
		os.MkdirAll(filepath.Join(d, "wireless"), 0o755)
		os.WriteFile(filepath.Join(d, "operstate"), []byte(estado+"\n"), 0o644)
	}
	return raiz
}

func enderecosFalsos(nome string) ([]net.Addr, error) {
	_, n, _ := net.ParseCIDR("10.0.0.209/24")
	n.IP = net.ParseIP("10.0.0.209").To4()
	return []net.Addr{n}, nil
}

func TestLeitorInterface(t *testing.T) {
	raiz := raizFalsa(t, rotasExemplo, map[string]string{"wlan0": "up", "enp3s0f0": "down"})
	l := &LeitorInterface{Raiz: raiz, Enderecos: enderecosFalsos}

	e, err := l.Ler("wlan0")
	if err != nil {
		t.Fatal(err)
	}
	if !e.Ativa || e.Tipo != "wifi" || e.IPv4 != "10.0.0.209/24" || e.Gateway.String() != "10.0.0.1" {
		t.Fatalf("estado errado: %+v", e)
	}
	e, _ = l.Ler("enp3s0f0")
	if e.Ativa {
		t.Fatal("cabo desligado aparece ativo")
	}
	if _, err := l.Ler("eth9"); err == nil {
		t.Fatal("interface inexistente deveria dar erro")
	}

	semRota := raizFalsa(t, "Iface\tDestination\n", map[string]string{"wlan0": "down"})
	l.Raiz = semRota
	if _, err := l.Ler(""); err == nil {
		t.Fatal("sem rota padrão e sem nome deveria dar erro")
	}
}

// rede falsa: cada sonda responde conforme os mapas; o que não está lá falha.
type redeFalsa struct {
	mu     sync.Mutex
	pingOK map[string]float64 // ip -> latência
	tcpOK  map[string]bool
	dnsOK  map[string]bool // servidor -> responde
	httpOK map[string]bool
	pings  int
}

func redeBoa() *redeFalsa {
	return &redeFalsa{
		pingOK: map[string]float64{"10.0.0.1": 7, "1.1.1.1": 20, "8.8.8.8": 41, "9.9.9.9": 30},
		tcpOK:  map[string]bool{},
		dnsOK:  map[string]bool{"8.8.8.8": true, "1.1.1.1": true},
		httpOK: map[string]bool{"https://a": true, "https://b": true},
	}
}

func (r *redeFalsa) Ping(_ context.Context, alvo netip.Addr, n int) ResultadoPing {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.pings++
	lat, ok := r.pingOK[alvo.String()]
	if !ok {
		return ResultadoPing{Enviados: n, Erro: "sem resposta"}
	}
	rtts := make([]time.Duration, n)
	for i := range rtts {
		rtts[i] = time.Duration(lat * float64(time.Millisecond))
	}
	return ResultadoPing{Enviados: n, Recebidos: n, RTTs: rtts}
}

func (r *redeFalsa) Conectar(_ context.Context, d netip.AddrPort) (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.tcpOK[d.Addr().String()] {
		return 15 * time.Millisecond, nil
	}
	return 0, errors.New("recusado")
}

func (r *redeFalsa) Resolver(_ context.Context, srv netip.AddrPort, _ string) (time.Duration, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.dnsOK[srv.Addr().String()] {
		return 25 * time.Millisecond, nil
	}
	return 0, errors.New("sem resposta")
}

func (r *redeFalsa) Testar(_ context.Context, url string) (time.Duration, int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.httpOK[url] {
		return 90 * time.Millisecond, 204, nil
	}
	return 0, 0, errors.New("tempo esgotado")
}

type armazemFalso struct {
	mu        sync.Mutex
	ciclos    []armazenamento.Ciclo
	quedas    map[int64]incidentes.Incidente
	proximo   int64
	batidas   int
	ultimaFim time.Time
	err       error
}

func novoArmazem() *armazemFalso { return &armazemFalso{quedas: map[int64]incidentes.Incidente{}} }

func (a *armazemFalso) SalvarCiclo(_ context.Context, c armazenamento.Ciclo) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.err != nil {
		return a.err
	}
	a.ciclos = append(a.ciclos, c)
	return nil
}
func (a *armazemFalso) IniciarExecucao(context.Context, time.Time, string) (int64, error) {
	return 1, a.err
}
func (a *armazemFalso) BaterExecucao(context.Context, int64, time.Time) error {
	a.batidas++
	return a.err
}
func (a *armazemFalso) UltimaExecucaoFim(context.Context) (time.Time, bool, error) {
	return a.ultimaFim, !a.ultimaFim.IsZero(), nil
}
func (a *armazemFalso) IncidenteAberto(context.Context) (*incidentes.Incidente, error) {
	for _, q := range a.quedas {
		if q.Fim == nil {
			return &q, nil
		}
	}
	return nil, nil
}
func (a *armazemFalso) SalvarIncidente(_ context.Context, q incidentes.Incidente) (int64, error) {
	if a.err != nil {
		return 0, a.err
	}
	if q.ID == 0 {
		a.proximo++
		q.ID = a.proximo
	}
	a.quedas[q.ID] = q
	return q.ID, nil
}

var wlan = EstadoInterface{Nome: "wlan0", Tipo: "wifi", Ativa: true, IPv4: "10.0.0.209/24", Gateway: netip.MustParseAddr("10.0.0.1")}

func novoMonitorTeste(iface *EstadoInterface, r *redeFalsa, a *armazemFalso) *Monitor {
	m := Novo(Opcoes{
		Intervalo:     time.Second,
		LerInterface:  func(string) (EstadoInterface, error) { return *iface, nil },
		Pinger:        r,
		Conector:      r,
		Resolvedor:    r,
		HTTP:          r,
		ServidoresDNS: func() []netip.AddrPort { return []netip.AddrPort{netip.MustParseAddrPort("8.8.8.8:53")} },
		URLs:          []string{"https://a", "https://b"},
		Armazem:       a,
		Log:           slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
	m.execucao = 1
	return m
}

func TestCicloTudoBem(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	if _, ok := m.Estado(); ok {
		t.Fatal("antes do primeiro ciclo não deveria ter estado")
	}
	e := m.ciclo(context.Background())
	if e.Diagnostico.Estado != diagnostico.OK || e.Gateway == nil || e.Gateway.MediaMS != 7 {
		t.Fatalf("estado: %+v", e.Diagnostico)
	}
	if len(e.Externos) != 3 || len(e.DNSSistema) != 1 || len(e.DNSExterno) != 1 || len(e.HTTP) != 2 {
		t.Fatalf("sondas: %+v", e)
	}
	c := a.ciclos[0]
	// interface + gateway + 3 externos + 2 DNS + 2 HTTP
	if c.Estado != "ok" || len(c.Verificacoes) != 9 {
		t.Fatalf("ciclo gravado: %s, %d verificações", c.Estado, len(c.Verificacoes))
	}
	if a.batidas != 1 {
		t.Fatal("deveria registrar o batimento do monitor")
	}
}

func TestQuedaDaInternetERecuperacao(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	base := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)
	n := 0
	m.agora = func() time.Time { return base.Add(time.Duration(n) * 10 * time.Second) }
	passo := func() Estado { e := m.ciclo(context.Background()); n++; return e }

	passo()
	// Provedor caiu: roteador responde, nada de fora.
	r.mu.Lock()
	r.pingOK = map[string]float64{"10.0.0.1": 7}
	r.dnsOK = map[string]bool{}
	r.httpOK = map[string]bool{}
	r.mu.Unlock()
	for i := 0; i < 3; i++ {
		e := passo()
		if e.Diagnostico.Causa != diagnostico.CausaExterno {
			t.Fatalf("causa: %s", e.Diagnostico.Causa)
		}
	}
	e, _ := m.Estado()
	if e.Queda == nil || !e.Queda.Inicio.Equal(base.Add(10*time.Second)) {
		t.Fatalf("queda deveria estar aberta desde o primeiro ciclo ruim: %+v", e.Queda)
	}
	if len(a.quedas) != 1 {
		t.Fatalf("queda gravada: %+v", a.quedas)
	}

	boa := redeBoa()
	r.mu.Lock()
	r.pingOK, r.dnsOK, r.httpOK = boa.pingOK, boa.dnsOK, boa.httpOK
	r.mu.Unlock()
	passo()
	passo()
	q := a.quedas[1]
	if q.Fim == nil || q.Fim.Sub(q.Inicio) != 30*time.Second {
		t.Fatalf("queda fechada errada: %+v", q)
	}
	if e, _ := m.Estado(); e.Queda != nil {
		t.Fatal("estado ainda mostra queda")
	}
}

func TestWifiCaidoNaoMedeORestante(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	iface := EstadoInterface{Nome: "wlan0", Tipo: "wifi"}
	m := novoMonitorTeste(&iface, r, a)
	e := m.ciclo(context.Background())
	if e.Diagnostico.Causa != diagnostico.CausaRedeLocal || r.pings != 0 {
		t.Fatalf("placa caída: causa %s, pings %d", e.Diagnostico.Causa, r.pings)
	}
	if len(a.ciclos[0].Verificacoes) != 1 {
		t.Fatal("só a verificação da interface deveria ser gravada")
	}
}

func TestPingBloqueadoUsaTCP(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	r.pingOK = map[string]float64{"10.0.0.1": 7} // operadora bloqueando ICMP
	r.tcpOK = map[string]bool{"1.1.1.1": true, "8.8.8.8": true, "9.9.9.9": true}
	iface := wlan
	e := novoMonitorTeste(&iface, r, a).ciclo(context.Background())
	if e.Diagnostico.Estado == diagnostico.Fora {
		t.Fatalf("ping bloqueado não é queda: %+v", e.Diagnostico)
	}
	if !e.Externos[0].ViaTCP {
		t.Fatal("deveria marcar que foi por TCP")
	}
}

func TestBancoForaNaoPara(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	a.err = errors.New("banco fora")
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	m.ciclo(context.Background())
	if e, ok := m.Estado(); !ok || e.Gateway == nil {
		t.Fatal("banco fora não pode impedir o estado em memória")
	}
}

func TestRetomarFechaQuedaAntiga(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	inicio := time.Date(2026, 10, 29, 7, 0, 0, 0, time.UTC)
	a.quedas[5] = incidentes.Incidente{ID: 5, Inicio: inicio, Causa: diagnostico.CausaExterno}
	a.ultimaFim = inicio.Add(2 * time.Minute)
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	m.retomar(context.Background())
	q := a.quedas[5]
	if q.Fim == nil || !q.Fim.Equal(a.ultimaFim) || !strings.Contains(q.Detalhe, "monitor parou") {
		t.Fatalf("queda antiga: %+v", q)
	}
}

func TestLerServidoresDNS(t *testing.T) {
	s := lerServidores(strings.NewReader("# x\nnameserver 8.8.8.8\nnameserver fe80::1%wlan0\nnameserver 1.1.1.1\nsearch .\n"))
	if len(s) != 2 || s[0].String() != "8.8.8.8:53" || s[1].String() != "1.1.1.1:53" {
		t.Fatalf("servidores: %v", s)
	}
}

// Ping de verdade no próprio PC: confere o socket sem privilégio.
func TestPingLocalReal(t *testing.T) {
	p := PingerICMP{Intervalo: 20 * time.Millisecond, Espera: time.Second}
	r := p.Ping(context.Background(), netip.MustParseAddr("127.0.0.1"), 3)
	if strings.Contains(r.Erro, "permissão") || strings.Contains(r.Erro, "socket") {
		t.Skipf("ping sem privilégio indisponível aqui: %s", r.Erro)
	}
	if r.Enviados != 3 || r.Recebidos != 3 || r.Perda() != 0 {
		t.Fatalf("ping local: %+v", r)
	}
}

func TestPingCancelado(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := PingerICMP{Intervalo: time.Second, Espera: 5 * time.Second}
	inicio := time.Now()
	p.Ping(ctx, netip.MustParseAddr("192.0.2.1"), 3) // TEST-NET: nunca responde
	if time.Since(inicio) > 2*time.Second {
		t.Fatal("ping cancelado não respeitou o contexto")
	}
}

func TestConfigurarTrocaOsAlvos(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	aj := ajustes.Padrao(10 * time.Second)
	aj.Externos = []string{"1.1.1.1"}
	aj.URLs = nil
	aj.IntervaloS = 30
	m.Configurar(aj)
	e := m.ciclo(context.Background())
	if len(e.Externos) != 1 || len(e.HTTP) != 0 {
		t.Fatalf("alvos novos não aplicados: %d externos, %d sites", len(e.Externos), len(e.HTTP))
	}
	if m.alvos().intervalo != 30*time.Second {
		t.Fatal("intervalo não aplicado")
	}
}

func TestTesteDeVelocidadeNaoViraLentidao(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	r.pingOK["1.1.1.1"], r.pingOK["8.8.8.8"], r.pingOK["9.9.9.9"] = 400, 450, 500 // fila cheia pelo teste
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	if e := m.ciclo(context.Background()); e.Diagnostico.Causa != diagnostico.CausaLatencia {
		t.Fatalf("sem teste rodando deveria ser lentidão: %s", e.Diagnostico.Causa)
	}
	m.op.Ocupado = func() bool { return true }
	e := m.ciclo(context.Background())
	if e.Diagnostico.Estado != diagnostico.OK || !strings.Contains(e.Diagnostico.Detalhe, "teste de velocidade") {
		t.Fatalf("durante o teste: %+v", e.Diagnostico)
	}
}

func TestQuedaDuranteTesteAindaConta(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	r.pingOK = map[string]float64{"10.0.0.1": 7}
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	m.op.Ocupado = func() bool { return true }
	if e := m.ciclo(context.Background()); e.Diagnostico.Estado != diagnostico.Fora {
		t.Fatalf("internet fora continua sendo queda durante o teste: %+v", e.Diagnostico)
	}
}

func TestCicloDuranteBuscaDeWifiNaoConta(t *testing.T) {
	r, a := redeBoa(), novoArmazem()
	iface := wlan
	m := novoMonitorTeste(&iface, r, a)
	m.ciclo(context.Background()) // um ciclo bom antes
	buscando := true
	m.op.Pausado = func() bool { return buscando }
	r.mu.Lock()
	r.dnsOK = map[string]bool{} // durante a busca, o DNS falha
	r.mu.Unlock()
	for i := 0; i < 4; i++ {
		e := m.ciclo(context.Background())
		if e.Diagnostico.Estado != diagnostico.OK {
			t.Fatalf("a tela deveria manter o último estado bom: %+v", e.Diagnostico)
		}
	}
	if len(a.ciclos) != 1 || len(a.quedas) != 0 {
		t.Fatalf("ciclos durante a busca não podem ser gravados nem virar queda: %d ciclos, %d quedas", len(a.ciclos), len(a.quedas))
	}
	buscando = false
	if e := m.ciclo(context.Background()); e.Diagnostico.Causa != diagnostico.CausaDNS {
		t.Fatalf("depois da busca, falha de DNS volta a contar: %+v", e.Diagnostico)
	}
}
