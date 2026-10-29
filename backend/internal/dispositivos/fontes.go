package dispositivos

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Vizinho é uma entrada da tabela de vizinhos do kernel (ARP).
type Vizinho struct {
	IP     netip.Addr
	MAC    net.HardwareAddr
	Estado string // REACHABLE, STALE, DELAY...
}

// Confirmado diz se o aparelho respondeu há pouco (~30 s). STALE é só
// lembrança: o kernel guarda entradas velhas por horas.
func (v Vizinho) Confirmado() bool {
	switch v.Estado {
	case "REACHABLE", "DELAY", "PROBE", "PERMANENT":
		return true
	}
	return false
}

// LerVizinhos pergunta ao kernel pela tabela de vizinhos com o estado de cada
// entrada (`ip -j neigh`, comando fixo; só a interface entra, já validada).
// Ler a tabela não envia nada na rede.
func LerVizinhos(ctx context.Context, iface string) ([]Vizinho, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "ip", "-j", "-4", "neigh", "show", "dev", iface)
	cmd.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin"}
	saida, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("ler a tabela de vizinhos: %w", err)
	}
	return analisarNeigh(saida)
}

func analisarNeigh(j []byte) ([]Vizinho, error) {
	var linhas []struct {
		Dst    string   `json:"dst"`
		LLAddr string   `json:"lladdr"`
		State  []string `json:"state"`
	}
	if err := json.Unmarshal(j, &linhas); err != nil {
		return nil, err
	}
	var vs []Vizinho
	for _, l := range linhas {
		ip, err1 := netip.ParseAddr(l.Dst)
		mac, err2 := net.ParseMAC(l.LLAddr)
		if err1 != nil || err2 != nil || len(l.State) == 0 {
			continue
		}
		vs = append(vs, Vizinho{IP: ip, MAC: mac, Estado: l.State[0]})
	}
	return vs, nil
}

// MACAleatorio: bit "administrado localmente" ligado. Celulares usam por rede,
// para não serem rastreados; nesses o fabricante não diz nada.
func MACAleatorio(mac net.HardwareAddr) bool {
	return len(mac) > 0 && mac[0]&0x02 != 0
}

// Fabricantes consulta a lista OUI da IEEE (pacote hwdata) só para os prefixos
// pedidos, guardando o que já achou. Não carrega os 6 MB na memória.
type Fabricantes struct {
	Arquivo string
	mu      sync.Mutex
	cache   map[string]string // prefixo "54-6C-AC" → nome ("" = não achou)
}

func NovaFabricantes(arquivo string) *Fabricantes {
	return &Fabricantes{Arquivo: arquivo, cache: map[string]string{}}
}

func prefixo(mac net.HardwareAddr) string {
	if len(mac) < 3 {
		return ""
	}
	return strings.ToUpper(strings.ReplaceAll(mac[:3].String(), ":", "-"))
}

// Buscar devolve o fabricante de cada MAC (aleatório fica sem).
func (f *Fabricantes) Buscar(macs []net.HardwareAddr) map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	faltam := map[string]bool{}
	for _, m := range macs {
		if p := prefixo(m); p != "" && !MACAleatorio(m) {
			if _, ok := f.cache[p]; !ok {
				faltam[p] = true
			}
		}
	}
	if len(faltam) > 0 {
		achados := f.varrerArquivo(faltam)
		for p := range faltam {
			f.cache[p] = achados[p]
		}
	}
	r := map[string]string{}
	for _, m := range macs {
		if !MACAleatorio(m) {
			r[m.String()] = f.cache[prefixo(m)]
		}
	}
	return r
}

func (f *Fabricantes) varrerArquivo(faltam map[string]bool) map[string]string {
	r := map[string]string{}
	arq, err := os.Open(f.Arquivo)
	if err != nil {
		return r // sem a lista (ex.: Pi sem hwdata): fica sem fabricante
	}
	defer arq.Close()
	sc := bufio.NewScanner(arq)
	for sc.Scan() && len(r) < len(faltam) {
		l := sc.Text()
		if len(l) < 9 || l[2] != '-' || !strings.Contains(l, "(hex)") {
			continue
		}
		if p := l[:8]; faltam[p] {
			_, nome, _ := strings.Cut(l, "(hex)")
			r[p] = strings.TrimSpace(nome)
		}
	}
	return r
}

// NomeReverso pergunta ao DNS do roteador o nome de um IP. O roteador
// costuma responder com o nome que o aparelho informou ao pegar o IP (DHCP).
func NomeReverso(ctx context.Context, servidor netip.AddrPort, ip netip.Addr) string {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", servidor.String())
		},
	}
	ctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()
	nomes, err := r.LookupAddr(ctx, ip.String())
	if err != nil || len(nomes) == 0 {
		return ""
	}
	n := strings.TrimSuffix(nomes[0], ".")
	n = strings.TrimSuffix(n, ".local")
	n = strings.TrimSuffix(n, ".lan")
	if len(n) > 63 {
		n = n[:63]
	}
	return n
}
