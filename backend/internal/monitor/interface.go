package monitor

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// EstadoInterface é o que o kernel diz da placa de rede usada para sair.
type EstadoInterface struct {
	Nome    string     `json:"nome"`
	Tipo    string     `json:"tipo"` // wifi, cabo ou outro
	Ativa   bool       `json:"ativa"`
	IPv4    string     `json:"ipv4,omitempty"`
	Gateway netip.Addr `json:"gateway"`
}

// TemRota diz se há rota padrão por esta interface.
func (e EstadoInterface) TemRota() bool { return e.Gateway.IsValid() }

type rota struct {
	iface   string
	gateway netip.Addr
	metrica int
}

const (
	rtfUp      = 0x1
	rtfGateway = 0x2
)

// analisarRotas lê o formato de /proc/net/route e devolve só as rotas padrão ativas.
func analisarRotas(r io.Reader) ([]rota, error) {
	var rotas []rota
	sc := bufio.NewScanner(r)
	primeira := true
	for sc.Scan() {
		if primeira { // cabeçalho
			primeira = false
			continue
		}
		c := strings.Fields(sc.Text())
		if len(c) < 8 {
			continue
		}
		destino, err1 := strconv.ParseUint(c[1], 16, 32)
		gw, err2 := strconv.ParseUint(c[2], 16, 32)
		flags, err3 := strconv.ParseUint(c[3], 16, 16)
		metrica, err4 := strconv.Atoi(c[6])
		mascara, err5 := strconv.ParseUint(c[7], 16, 32)
		if err := errors.Join(err1, err2, err3, err4, err5); err != nil {
			return nil, fmt.Errorf("linha de rota inválida %q: %w", sc.Text(), err)
		}
		if destino != 0 || mascara != 0 || flags&rtfUp == 0 || flags&rtfGateway == 0 {
			continue
		}
		// O kernel escreve o endereço em ordem de máquina (little-endian no x86 e no Pi).
		ip := netip.AddrFrom4([4]byte{byte(gw), byte(gw >> 8), byte(gw >> 16), byte(gw >> 24)})
		rotas = append(rotas, rota{iface: c[0], gateway: ip, metrica: metrica})
	}
	return rotas, sc.Err()
}

// rotaPadrao escolhe a de menor métrica; com nome, só daquela interface.
func rotaPadrao(rotas []rota, nome string) (rota, bool) {
	var melhor rota
	achou := false
	for _, r := range rotas {
		if nome != "" && r.iface != nome {
			continue
		}
		if !achou || r.metrica < melhor.metrica {
			melhor, achou = r, true
		}
	}
	return melhor, achou
}

// LeitorInterface lê o estado da interface a partir de /proc e /sys.
// Raiz existe para os testes apontarem para uma pasta falsa.
type LeitorInterface struct {
	Raiz      string
	Enderecos func(nome string) ([]net.Addr, error)
}

func NovoLeitorInterface() *LeitorInterface {
	return &LeitorInterface{Raiz: "/", Enderecos: enderecosDoSistema}
}

func enderecosDoSistema(nome string) ([]net.Addr, error) {
	i, err := net.InterfaceByName(nome)
	if err != nil {
		return nil, err
	}
	return i.Addrs()
}

// Ler devolve o estado. nome vazio = a interface da rota padrão.
// Erro só quando não há como saber nada; interface sem rota não é erro.
func (l *LeitorInterface) Ler(nome string) (EstadoInterface, error) {
	f, err := os.Open(filepath.Join(l.Raiz, "proc/net/route"))
	if err != nil {
		return EstadoInterface{Nome: nome}, err
	}
	rotas, err := analisarRotas(f)
	f.Close()
	if err != nil {
		return EstadoInterface{Nome: nome}, err
	}
	r, temRota := rotaPadrao(rotas, nome)
	if nome == "" {
		if !temRota {
			return EstadoInterface{}, errors.New("nenhuma rota padrão: este aparelho não está ligado a nenhuma rede")
		}
		nome = r.iface
	}

	e := EstadoInterface{Nome: nome, Tipo: l.tipo(nome)}
	if temRota {
		e.Gateway = r.gateway
	}
	sys := filepath.Join(l.Raiz, "sys/class/net", nome)
	estado, err := os.ReadFile(filepath.Join(sys, "operstate"))
	if err != nil {
		return e, fmt.Errorf("interface %s não existe", nome)
	}
	s := strings.TrimSpace(string(estado))
	e.Ativa = s == "up" || s == "unknown"

	if addrs, err := l.Enderecos(nome); err == nil {
		for _, a := range addrs {
			if n, ok := a.(*net.IPNet); ok && n.IP.To4() != nil {
				e.IPv4 = n.String()
				break
			}
		}
	}
	return e, nil
}

func (l *LeitorInterface) tipo(nome string) string {
	sys := filepath.Join(l.Raiz, "sys/class/net", nome)
	if _, err := os.Stat(filepath.Join(sys, "wireless")); err == nil {
		return "wifi"
	}
	if _, err := os.Stat(filepath.Join(sys, "device")); err == nil {
		return "cabo"
	}
	return "outro"
}
