package diagnostico

import "testing"

func ok(alvo string, ms float64) Sonda  { return Sonda{Alvo: alvo, OK: true, LatenciaMS: ms} }
func falha(alvo string) Sonda           { return Sonda{Alvo: alvo, Perda: 1} }
func comPerda(s Sonda, p float64) Sonda { s.Perda = p; return s }

// base: tudo funcionando, como na medição de 2026-10-29.
func base() Entrada {
	return Entrada{
		InterfaceOK:   true,
		GatewayMedido: true,
		Gateway:       ok("10.0.0.1", 7),
		Externos:      []Sonda{ok("1.1.1.1", 20), ok("8.8.8.8", 41), ok("9.9.9.9", 30)},
		DNSSistema:    []Sonda{ok("8.8.8.8", 30)},
		DNSExterno:    []Sonda{ok("1.1.1.1", 25)},
		HTTP:          []Sonda{ok("google", 100), ok("cloudflare", 90)},
	}
}

func TestDiagnosticar(t *testing.T) {
	casos := []struct {
		nome   string
		mudar  func(*Entrada)
		estado Estado
		causa  Causa
	}{
		{"tudo normal", func(*Entrada) {}, OK, CausaNenhuma},
		{"wi-fi caiu", func(e *Entrada) {
			e.InterfaceOK, e.InterfaceProblema = false, "interface desligada"
			// tudo o resto falha junto, mas a causa é a placa
			e.GatewayMedido = false
			e.Externos = []Sonda{falha("1.1.1.1")}
		}, Fora, CausaRedeLocal},
		{"roteador mudo", func(e *Entrada) {
			e.Gateway = falha("10.0.0.1")
			e.Externos = []Sonda{falha("1.1.1.1"), falha("8.8.8.8")}
		}, Fora, CausaGateway},
		{"internet fora com roteador ok", func(e *Entrada) {
			e.Externos = []Sonda{falha("1.1.1.1"), falha("8.8.8.8"), falha("9.9.9.9")}
			e.DNSSistema = []Sonda{falha("8.8.8.8")}
			e.DNSExterno = []Sonda{falha("1.1.1.1")}
		}, Fora, CausaExterno},
		{"um IP externo fora não é queda", func(e *Entrada) {
			e.Externos[1] = falha("8.8.8.8")
		}, Degradado, CausaPerda}, // 1 de 3 sem resposta = 33% de perda média
		{"só o DNS configurado falha", func(e *Entrada) {
			e.DNSSistema = []Sonda{falha("8.8.8.8")}
		}, Fora, CausaDNSConfigurado},
		{"nenhum DNS responde", func(e *Entrada) {
			e.DNSSistema = []Sonda{falha("8.8.8.8")}
			e.DNSExterno = []Sonda{falha("1.1.1.1")}
		}, Fora, CausaDNS},
		{"um site fora", func(e *Entrada) {
			e.HTTP[1] = falha("cloudflare")
		}, Degradado, CausaServico},
		{"internet lenta", func(e *Entrada) {
			e.Externos = []Sonda{ok("1.1.1.1", 300), ok("8.8.8.8", 250)}
		}, Degradado, CausaLatencia},
		{"perda alta", func(e *Entrada) {
			e.Externos = []Sonda{comPerda(ok("1.1.1.1", 20), 0.4), comPerda(ok("8.8.8.8", 40), 0.2)}
		}, Degradado, CausaPerda},
		{"perda pequena tolerada", func(e *Entrada) {
			e.Externos = []Sonda{comPerda(ok("1.1.1.1", 20), 0.05), ok("8.8.8.8", 40)}
		}, OK, CausaNenhuma},
		{"sem rota: gateway não medido, externos falham", func(e *Entrada) {
			e.GatewayMedido = false
			e.Externos = []Sonda{falha("1.1.1.1")}
		}, Fora, CausaExterno},
	}
	for _, c := range casos {
		e := base()
		c.mudar(&e)
		r := Diagnosticar(e, LimitesPadrao)
		if r.Estado != c.estado || r.Causa != c.causa {
			t.Errorf("%s: esperava %s/%s, veio %s/%s (%s)", c.nome, c.estado, c.causa, r.Estado, r.Causa, r.Detalhe)
		}
	}
}

func TestTodasAsCausasTemTexto(t *testing.T) {
	for _, c := range []Causa{CausaNenhuma, CausaRedeLocal, CausaGateway, CausaExterno, CausaDNSConfigurado, CausaDNS, CausaServico, CausaLatencia, CausaPerda} {
		if Texto(c) == string(c) {
			t.Errorf("causa sem texto: %s", c)
		}
	}
	if Texto("operadora") != "operadora" {
		t.Error("causa desconhecida deveria voltar o código")
	}
}
