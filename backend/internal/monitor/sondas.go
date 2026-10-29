package monitor

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"os"
	"strings"
	"time"
)

// Resolvedor pergunta um nome direto a um servidor DNS, sem cache no meio.
type Resolvedor interface {
	Resolver(ctx context.Context, servidor netip.AddrPort, nome string) (time.Duration, error)
}

type ResolvedorUDP struct{}

func (ResolvedorUDP) Resolver(ctx context.Context, servidor netip.AddrPort, nome string) (time.Duration, error) {
	r := &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, _, _ string) (net.Conn, error) {
			var d net.Dialer
			return d.DialContext(ctx, "udp", servidor.String())
		},
	}
	if !strings.HasSuffix(nome, ".") {
		nome += "." // nome completo: não tenta os domínios de busca
	}
	inicio := time.Now()
	ips, err := r.LookupNetIP(ctx, "ip4", nome)
	d := time.Since(inicio)
	if err != nil {
		return d, simplificarErroDNS(err)
	}
	if len(ips) == 0 {
		return d, errors.New("resposta vazia")
	}
	return d, nil
}

func simplificarErroDNS(err error) error {
	var de *net.DNSError
	if errors.As(err, &de) {
		switch {
		case de.IsTimeout:
			return errors.New("sem resposta (tempo esgotado)")
		case de.IsNotFound:
			return errors.New("nome não encontrado")
		}
		return errors.New(de.Err)
	}
	return err
}

// ServidoresDNS lê os servidores que o sistema usa de verdade. Com o
// systemd-resolved, /etc/resolv.conf só aponta para 127.0.0.53 (o cache);
// os servidores reais ficam em /run/systemd/resolve/resolv.conf.
func ServidoresDNS() []netip.AddrPort {
	for _, arq := range []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"} {
		f, err := os.Open(arq)
		if err != nil {
			continue
		}
		s := lerServidores(f)
		f.Close()
		if len(s) > 0 {
			return s
		}
	}
	return nil
}

func lerServidores(r io.Reader) []netip.AddrPort {
	var s []netip.AddrPort
	sc := bufio.NewScanner(r)
	for sc.Scan() && len(s) < 3 {
		c := strings.Fields(sc.Text())
		if len(c) < 2 || c[0] != "nameserver" {
			continue
		}
		ip, err := netip.ParseAddr(strings.Split(c[1], "%")[0])
		if err != nil || !ip.Is4() {
			continue
		}
		s = append(s, netip.AddrPortFrom(ip, 53))
	}
	return s
}

// TestadorHTTP abre a URL do zero a cada vez (sem reaproveitar conexão),
// para medir o caminho inteiro: DNS, TCP, TLS e resposta.
type TestadorHTTP interface {
	Testar(ctx context.Context, url string) (dur time.Duration, status int, err error)
}

type ClienteHTTP struct {
	c      *http.Client
	agente string
}

func NovoClienteHTTP(versao string) *ClienteHTTP {
	return &ClienteHTTP{
		agente: "NetGuard/" + versao,
		c: &http.Client{
			Transport: &http.Transport{
				DisableKeepAlives:     true,
				TLSHandshakeTimeout:   4 * time.Second,
				ResponseHeaderTimeout: 4 * time.Second,
			},
			CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		},
	}
}

func (h *ClienteHTTP) Testar(ctx context.Context, url string) (time.Duration, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, 0, err
	}
	req.Header.Set("User-Agent", h.agente)
	inicio := time.Now()
	resp, err := h.c.Do(req)
	if err != nil {
		return time.Since(inicio), 0, simplificarErroHTTP(err)
	}
	io.Copy(io.Discard, io.LimitReader(resp.Body, 64<<10))
	resp.Body.Close()
	d := time.Since(inicio)
	if resp.StatusCode >= 400 {
		return d, resp.StatusCode, fmt.Errorf("respondeu %d", resp.StatusCode)
	}
	return d, resp.StatusCode, nil
}

func simplificarErroHTTP(err error) error {
	var de *net.DNSError
	switch {
	case errors.As(err, &de):
		return errors.New("DNS não resolveu o endereço")
	case errors.Is(err, context.DeadlineExceeded), os.IsTimeout(err):
		return errors.New("tempo esgotado")
	}
	var oe *net.OpError
	if errors.As(err, &oe) {
		return fmt.Errorf("conexão falhou (%s)", oe.Op)
	}
	return err
}

// Conector abre uma conexão TCP: reserva para quando o ping é bloqueado no caminho.
type Conector interface {
	Conectar(ctx context.Context, destino netip.AddrPort) (time.Duration, error)
}

type ConectorTCP struct{}

func (ConectorTCP) Conectar(ctx context.Context, destino netip.AddrPort) (time.Duration, error) {
	var d net.Dialer
	inicio := time.Now()
	c, err := d.DialContext(ctx, "tcp", destino.String())
	dur := time.Since(inicio)
	if err != nil {
		return dur, err
	}
	c.Close()
	return dur, nil
}
