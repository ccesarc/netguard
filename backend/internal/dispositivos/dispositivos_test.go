package dispositivos

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Saída real de `ip -j -4 neigh show dev wlan0` deste PC em 2026-10-29 (resumida).
const neighExemplo = `[{"dst":"10.0.0.242","state":["FAILED"]},
{"dst":"10.0.0.1","lladdr":"54:6c:ac:12:34:43","state":["REACHABLE"]},
{"dst":"10.0.0.28","lladdr":"66:a6:27:12:34:7f","state":["STALE"]},
{"dst":"10.0.0.30","lladdr":"e6:09:58:12:34:33","state":["REACHABLE"]},
{"dst":"10.0.0.133","lladdr":"f8:01:b4:12:34:b4","state":["DELAY"]}]`

func TestAnalisarNeigh(t *testing.T) {
	vs, err := analisarNeigh([]byte(neighExemplo))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 4 {
		t.Fatalf("entradas com MAC: %d", len(vs))
	}
	confirmados := 0
	for _, v := range vs {
		if v.Confirmado() {
			confirmados++
		}
	}
	if confirmados != 3 {
		t.Fatalf("STALE não pode contar como presente: %d confirmados", confirmados)
	}
}

func TestMACAleatorio(t *testing.T) {
	casos := map[string]bool{"54:6c:ac:12:34:43": false, "e6:09:58:12:34:33": true, "56:e2:c7:12:34:ac": true, "f8:01:b4:12:34:b4": false}
	for m, esperado := range casos {
		mac, _ := net.ParseMAC(m)
		if MACAleatorio(mac) != esperado {
			t.Errorf("%s: esperava %v", m, esperado)
		}
	}
}

func TestFabricantes(t *testing.T) {
	arq := filepath.Join(t.TempDir(), "oui.txt")
	os.WriteFile(arq, []byte("OUI/MA-L\n\n54-6C-AC   (hex)\t\tIntelbras\n546CAC     (base 16)\t\tIntelbras\nF8-01-B4   (hex)\t\tLG Electronics\n"), 0o644)
	f := NovaFabricantes(arq)
	m1, _ := net.ParseMAC("54:6c:ac:12:34:43")
	m2, _ := net.ParseMAC("f8:01:b4:12:34:b4")
	m3, _ := net.ParseMAC("e6:09:58:12:34:33") // aleatório
	m4, _ := net.ParseMAC("00:11:22:33:44:55") // não está na lista
	r := f.Buscar([]net.HardwareAddr{m1, m2, m3, m4})
	if r[m1.String()] != "Intelbras" || r[m2.String()] != "LG Electronics" || r[m3.String()] != "" || r[m4.String()] != "" {
		t.Fatalf("fabricantes: %v", r)
	}
	os.Remove(arq)
	if f.Buscar([]net.HardwareAddr{m1})[m1.String()] != "Intelbras" {
		t.Fatal("deveria usar o que já achou")
	}
	if len(NovaFabricantes("/nao/existe").Buscar([]net.HardwareAddr{m1})) != 1 {
		t.Fatal("sem arquivo: devolve vazio, não quebra")
	}
}

type armazemFalso struct {
	mu       sync.Mutex
	obs      [][]Observacao
	retratos []bool
	novo     int
	primeira bool
}

func (a *armazemFalso) RegistrarObservacoes(_ context.Context, _ time.Time, obs []Observacao, _ time.Duration, retrato bool) (int, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.obs = append(a.obs, obs)
	a.retratos = append(a.retratos, retrato)
	return a.novo, nil
}

func (a *armazemFalso) PrimeiraVarreduraFeita(context.Context) (bool, error) { return a.primeira, nil }
func (a *armazemFalso) MarcarPrimeiraVarredura(context.Context) error        { a.primeira = true; return nil }

type fabFalso struct{}

func (fabFalso) Buscar(macs []net.HardwareAddr) map[string]string {
	r := map[string]string{}
	for _, m := range macs {
		if m.String() == "54:6c:ac:12:34:43" {
			r[m.String()] = "Intelbras"
		}
	}
	return r
}

func novoColetorTeste(arm *armazemFalso, ativa bool, cutucados *[]netip.Addr) *Coletor {
	proprio, _ := net.ParseMAC("14:bd:61:12:34:c4")
	var mu sync.Mutex
	return NovoColetor(Opcoes{
		Rede: func() (Rede, bool) {
			return Rede{Iface: "wlan0", Endereco: netip.MustParsePrefix("10.0.0.209/24"), Gateway: netip.MustParseAddr("10.0.0.1"), MAC: proprio}, true
		},
		LerVizinhos: func(context.Context, string) ([]Vizinho, error) { return analisarNeigh([]byte(neighExemplo)) },
		Cutucar: func(_ context.Context, ip netip.Addr) {
			mu.Lock()
			*cutucados = append(*cutucados, ip)
			mu.Unlock()
		},
		NomeReverso: func(_ context.Context, _ netip.AddrPort, ip netip.Addr) string {
			if ip.String() == "10.0.0.30" {
				return "Notebook"
			}
			return ""
		},
		Fabricantes:    fabFalso{},
		VarreduraAtiva: func() bool { return ativa },
		Armazem:        arm,
		Log:            slog.New(slog.NewTextHandler(io.Discard, nil)),
	})
}

func TestColetaPassiva(t *testing.T) {
	arm := &armazemFalso{}
	var cut []netip.Addr
	c := novoColetorTeste(arm, false, &cut)
	c.coletar(context.Background(), false)
	if len(cut) != 0 {
		t.Fatal("coleta passiva não pode mandar nada na rede")
	}
	obs := arm.obs[0]
	// 3 confirmados (roteador, Mac, TV) + este aparelho; o STALE fica de fora
	if len(obs) != 4 {
		t.Fatalf("observações: %+v", obs)
	}
	por := map[string]Observacao{}
	for _, o := range obs {
		por[o.IP.String()] = o
	}
	if por["10.0.0.1"].Fabricante != "Intelbras" || por["10.0.0.30"].Nome != "Notebook" || !por["10.0.0.30"].Aleatorio {
		t.Fatalf("detalhes: %+v", por)
	}
	if !por["10.0.0.209"].EsteAparelho {
		t.Fatal("este aparelho deveria entrar na lista")
	}
	if _, tem := por["10.0.0.28"]; tem {
		t.Fatal("STALE entrou como presente")
	}
	if c.Estado().UltimaColeta == nil {
		t.Fatal("estado sem última coleta")
	}
}

func TestVarreduraAtiva(t *testing.T) {
	arm := &armazemFalso{}
	var cut []netip.Addr
	c := novoColetorTeste(arm, true, &cut)
	c.coletar(context.Background(), true)
	// /24: 254 endereços menos este aparelho
	if len(cut) != 253 {
		t.Fatalf("cutucados: %d", len(cut))
	}
	for _, ip := range cut {
		if ip.String() == "10.0.0.209" || ip.String() == "10.0.0.0" || ip.String() == "10.0.0.255" {
			t.Fatalf("não devia cutucar %s", ip)
		}
	}
	if e := c.Estado(); e.UltimaVarredura == nil || e.UltimaVarreduraInicio == nil || e.UltimaVarreduraInicio.After(*e.UltimaVarredura) {
		t.Fatalf("varredura não registrada direito: %+v", e)
	}
	// 253 endereços em blocos de 64 = 4 registros parciais + o final, todos retrato (1ª procura)
	if len(arm.obs) != 5 {
		t.Fatalf("registros durante a procura: %d", len(arm.obs))
	}
	for i, r := range arm.retratos {
		if !r {
			t.Fatalf("registro %d da primeira procura deveria ser retrato", i)
		}
	}
	if !arm.primeira {
		t.Fatal("primeira procura não foi marcada")
	}
	if c.Estado().Progresso != nil || c.Estado().Varrendo {
		t.Fatal("no fim não pode sobrar progresso")
	}
	arm.retratos = nil // segunda procura: não é mais retrato
	c.mu.Lock()
	c.ultimaVarredura, c.varrendo = time.Time{}, true
	c.mu.Unlock()
	c.coletar(context.Background(), true)
	for _, r := range arm.retratos {
		if r {
			t.Fatal("segunda procura não pode ser retrato")
		}
	}
	if err := c.VarrerAgora(context.Background()); err != ErrVarreduraCedo {
		t.Fatalf("logo depois: %v", err)
	}
}

func TestNomeGuardado(t *testing.T) {
	arm := &armazemFalso{}
	var cut []netip.Addr
	c := novoColetorTeste(arm, false, &cut)
	var perguntas atomic.Int32
	c.op.NomeReverso = func(context.Context, netip.AddrPort, netip.Addr) string { perguntas.Add(1); return "x" }
	c.coletar(context.Background(), false)
	antes := perguntas.Load()
	c.coletar(context.Background(), false)
	if perguntas.Load() != antes {
		t.Fatalf("deveria usar o nome guardado: %d → %d perguntas", antes, perguntas.Load())
	}
}

// Leitura de verdade deste PC; pula onde não houver `ip` ou wlan0.
func TestLeituraReal(t *testing.T) {
	vs, err := LerVizinhos(context.Background(), "wlan0")
	if err != nil {
		t.Skipf("sem tabela de vizinhos aqui: %v", err)
	}
	for _, v := range vs {
		if !v.IP.IsValid() || len(v.MAC) != 6 {
			t.Fatalf("entrada estranha: %+v", v)
		}
	}
}

func TestProgressoDuranteAProcura(t *testing.T) {
	arm := &armazemFalso{primeira: true}
	var cut []netip.Addr
	c := novoColetorTeste(arm, true, &cut)
	var vistos []Progresso
	var mu sync.Mutex
	cutucar := c.op.Cutucar
	c.op.Cutucar = func(ctx context.Context, ip netip.Addr) {
		cutucar(ctx, ip)
		if ip.As4()[3]%64 == 0 { // de vez em quando, olha o estado como a tela olharia
			if p := c.Estado().Progresso; p != nil {
				mu.Lock()
				vistos = append(vistos, *p)
				mu.Unlock()
			}
		}
	}
	c.coletar(context.Background(), true)
	if len(vistos) == 0 {
		t.Fatal("a tela nunca viu progresso")
	}
	for _, p := range vistos {
		if p.Total != 253 || p.Feitos > p.Total {
			t.Fatalf("progresso estranho: %+v", p)
		}
	}
	if vistos[len(vistos)-1].Achados == 0 {
		t.Fatal("deveria contar quem já respondeu durante a procura")
	}
}
