package wifi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
)

// Saída real deste PC em 2026-10-29, mais duas redes inventadas para o teste.
const saidaExemplo = `*:MINHA_REDE_5G:48:5240 MHz:1170 Mbit/s:79:WPA2
 :VIZINHO\:5G:48:5240 MHz:540 Mbit/s:31:WPA2
 :MINHA_REDE:6:2437 MHz:130 Mbit/s:82:WPA2
`

func TestAnalisar(t *testing.T) {
	a, err := analisar(saidaExemplo)
	if err != nil {
		t.Fatal(err)
	}
	if a.SSID != "MINHA_REDE_5G" || a.Canal != 48 || a.FreqMHz != 5240 || a.TaxaMbps != 1170 || a.Sinal != 79 || a.Seguranca != "WPA2" {
		t.Fatalf("leitura: %+v", a)
	}
	if a.RedesVistas != 2 || a.RedesMesmoCanal != 1 {
		t.Fatalf("vizinhos: vistas=%d mesmo canal=%d", a.RedesVistas, a.RedesMesmoCanal)
	}
}

func TestSSIDComDoisPontos(t *testing.T) {
	c := campos(`*:Casa\:2\\Andar:6:2437 MHz:65 Mbit/s:40:WPA2`)
	if c[1] != `Casa:2\Andar` || len(c) != 7 {
		t.Fatalf("campos: %q", c)
	}
}

func TestNaoConectado(t *testing.T) {
	if _, err := analisar(" :X:1:2412 MHz:54 Mbit/s:20:WPA2\n"); !errors.Is(err, ErrNaoConectado) {
		t.Fatalf("sem '*': %v", err)
	}
	if _, err := analisar(""); !errors.Is(err, ErrNaoConectado) {
		t.Fatalf("vazio: %v", err)
	}
}

type leitorFalso struct {
	a   Amostra
	err error
}

func (l leitorFalso) Ler(context.Context, string) (Amostra, error) { return l.a, l.err }

type armazemFalso struct{ n int }

func (a *armazemFalso) SalvarWifi(context.Context, Amostra) error { a.n++; return nil }

func TestColetor(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	arm := &armazemFalso{}

	c := NovoColetor(leitorFalso{a: Amostra{SSID: "x", Sinal: 70}}, arm, func() (string, string) { return "wlan0", "wifi" }, log)
	c.coletar(context.Background())
	if a, _ := c.Ultima(); a == nil || a.Sinal != 70 || a.Em.IsZero() || arm.n != 1 {
		t.Fatalf("wifi: %+v gravadas=%d", a, arm.n)
	}

	c = NovoColetor(leitorFalso{}, arm, func() (string, string) { return "eth0", "cabo" }, log)
	c.coletar(context.Background())
	if a, m := c.Ultima(); a != nil || m == "" {
		t.Fatalf("no cabo não lê Wi-Fi: %+v %q", a, m)
	}

	c = NovoColetor(leitorFalso{err: errors.New("nmcli falhou")}, arm, func() (string, string) { return "wlan0", "wifi" }, log)
	c.coletar(context.Background())
	if a, m := c.Ultima(); a != nil || m == "" {
		t.Fatalf("erro de leitura: %+v %q", a, m)
	}
}

// Leitura de verdade deste PC; pula onde não houver nmcli ou Wi-Fi.
func TestLeituraReal(t *testing.T) {
	a, err := LeitorNmcli{}.Ler(context.Background(), "")
	if err != nil {
		t.Skipf("sem Wi-Fi para ler aqui: %v", err)
	}
	if a.SSID == "" || a.Sinal <= 0 || a.Canal <= 0 {
		t.Fatalf("leitura real estranha: %+v", a)
	}
}
