package monitor

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net"
	"net/netip"
	"syscall"
	"time"

	"golang.org/x/net/icmp"
	"golang.org/x/net/ipv4"
)

type ResultadoPing struct {
	Enviados  int
	Recebidos int
	RTTs      []time.Duration
	Erro      string
}

func (r ResultadoPing) Perda() float64 {
	if r.Enviados == 0 {
		return 1
	}
	return float64(r.Enviados-r.Recebidos) / float64(r.Enviados)
}

func (r ResultadoPing) Estatisticas() (minimo, media, maximo time.Duration) {
	if len(r.RTTs) == 0 {
		return 0, 0, 0
	}
	minimo, maximo = r.RTTs[0], r.RTTs[0]
	var soma time.Duration
	for _, d := range r.RTTs {
		minimo, maximo = min(minimo, d), max(maximo, d)
		soma += d
	}
	return minimo, soma / time.Duration(len(r.RTTs)), maximo
}

type Pinger interface {
	Ping(ctx context.Context, alvo netip.Addr, n int) ResultadoPing
}

// PingerICMP usa o socket de ping sem privilégio do Linux (udp4 +
// net.ipv4.ping_group_range). Não precisa de root nem de CAP_NET_RAW.
type PingerICMP struct {
	Intervalo time.Duration // entre um pacote e outro
	Espera    time.Duration // quanto esperar a última resposta
}

func (p PingerICMP) Ping(ctx context.Context, alvo netip.Addr, n int) ResultadoPing {
	var res ResultadoPing
	if !alvo.Is4() {
		res.Erro = "só IPv4 por enquanto"
		return res
	}
	conn, err := icmp.ListenPacket("udp4", "0.0.0.0")
	if err != nil {
		if errors.Is(err, syscall.EACCES) || errors.Is(err, syscall.EPERM) {
			res.Erro = "sem permissão para ping: ajuste net.ipv4.ping_group_range"
		} else {
			res.Erro = fmt.Sprintf("abrir socket de ping: %v", err)
		}
		return res
	}
	defer conn.Close()

	// O prazo geral vem antes da goroutine abaixo, senão poderia apagar o "pare agora".
	conn.SetReadDeadline(time.Now().Add(time.Duration(n)*p.Intervalo + p.Espera))
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	go func() {
		<-ctx.Done()
		conn.SetReadDeadline(time.Now()) // destrava o leitor
	}()

	destino := &net.UDPAddr{IP: net.IP(alvo.AsSlice())}
	base := rand.IntN(65536 - n)
	enviadoEm := make([]time.Time, n)
	recebido := make([]bool, n)

	type resposta struct {
		seq int
		em  time.Time
	}
	respostas := make(chan resposta, 2*n+8)
	go func() {
		defer close(respostas)
		buf := make([]byte, 1500)
		for {
			nr, origem, err := conn.ReadFrom(buf)
			if err != nil {
				return
			}
			em := time.Now()
			msg, err := icmp.ParseMessage(1, buf[:nr])
			if err != nil || msg.Type != ipv4.ICMPTypeEchoReply {
				continue
			}
			eco, ok := msg.Body.(*icmp.Echo)
			u, ok2 := origem.(*net.UDPAddr)
			if !ok || !ok2 || !u.IP.Equal(destino.IP) {
				continue
			}
			select {
			case respostas <- resposta{seq: eco.Seq, em: em}:
			default: // respostas demais (duplicadas): descarta
			}
		}
	}()

	for i := 0; i < n; i++ {
		msg := icmp.Message{Type: ipv4.ICMPTypeEcho, Body: &icmp.Echo{Seq: base + i, Data: []byte("netguard")}}
		b, _ := msg.Marshal(nil)
		enviadoEm[i] = time.Now()
		if _, err := conn.WriteTo(b, destino); err != nil {
			res.Erro = fmt.Sprintf("enviar: %v", err)
			enviadoEm[i] = time.Time{}
			break
		}
		res.Enviados++
		if i < n-1 {
			select {
			case <-ctx.Done():
				i = n
			case <-time.After(p.Intervalo):
			}
		}
	}
	if res.Enviados == 0 {
		cancel()
	}

	for r := range respostas {
		i := (r.seq - base) & 0xffff
		if i >= n || recebido[i] || enviadoEm[i].IsZero() {
			continue
		}
		recebido[i] = true
		res.Recebidos++
		res.RTTs = append(res.RTTs, r.em.Sub(enviadoEm[i]))
		if res.Recebidos == res.Enviados {
			cancel() // todas voltaram: não precisa esperar o prazo
		}
	}
	if res.Recebidos == 0 && res.Erro == "" && res.Enviados > 0 {
		res.Erro = "sem resposta"
	}
	return res
}
