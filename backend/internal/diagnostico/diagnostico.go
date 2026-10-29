// Package diagnostico decide o estado da rede e a causa provável a partir do
// resultado de um ciclo. Função pura: sem rede, sem banco, sem relógio.
//
// A ordem importa: de dentro para fora (este aparelho → roteador → internet →
// DNS → serviços). Sem acesso ao roteador não dá para separar provedor de
// roteador, então a causa externa nunca é "operadora".
package diagnostico

import (
	"fmt"
	"strings"
)

type Estado string

const (
	OK        Estado = "ok"
	Degradado Estado = "degradado"
	Fora      Estado = "fora"
)

type Causa string

const (
	CausaNenhuma        Causa = "ok"
	CausaRedeLocal      Causa = "rede_local"
	CausaGateway        Causa = "gateway"
	CausaExterno        Causa = "externo"
	CausaDNSConfigurado Causa = "dns_configurado"
	CausaDNS            Causa = "dns"
	CausaServico        Causa = "servico"
	CausaLatencia       Causa = "latencia_alta"
	CausaPerda          Causa = "perda_alta"
)

var textos = map[Causa]string{
	CausaNenhuma:        "Tudo normal",
	CausaRedeLocal:      "Rede local: este aparelho está sem rede (Wi-Fi ou cabo)",
	CausaGateway:        "Roteador não responde",
	CausaExterno:        "Sem internet: o roteador responde, mas nada de fora (provedor ou roteador)",
	CausaDNSConfigurado: "DNS configurado não responde (outro DNS responde)",
	CausaDNS:            "DNS não responde (nenhum DNS testado)",
	CausaServico:        "Um serviço específico está fora",
	CausaLatencia:       "Internet lenta (latência alta)",
	CausaPerda:          "Perdendo pacotes",
}

// Texto é a frase para mostrar na tela.
func Texto(c Causa) string {
	if t, ok := textos[c]; ok {
		return t
	}
	return string(c)
}

type Sonda struct {
	Alvo       string
	OK         bool
	LatenciaMS float64 // média; só vale com OK
	Perda      float64 // 0..1; só para ping
}

type Entrada struct {
	InterfaceOK       bool
	InterfaceProblema string

	GatewayMedido bool
	Gateway       Sonda

	Externos   []Sonda // IPs públicos (ping ou TCP 443)
	DNSSistema []Sonda // servidores configurados no aparelho
	DNSExterno []Sonda // servidor independente, para comparar
	HTTP       []Sonda
}

type Limites struct {
	LatenciaMS float64 // média dos externos acima disso = lenta
	Perda      float64 // perda média dos externos acima disso
}

var LimitesPadrao = Limites{LatenciaMS: 150, Perda: 0.1}

type Resultado struct {
	Estado  Estado
	Causa   Causa
	Detalhe string
}

func Diagnosticar(e Entrada, l Limites) Resultado {
	if !e.InterfaceOK {
		return Resultado{Fora, CausaRedeLocal, e.InterfaceProblema}
	}
	if e.GatewayMedido && !e.Gateway.OK {
		return Resultado{Fora, CausaGateway, e.Gateway.Alvo}
	}
	if len(e.Externos) > 0 && nenhumOK(e.Externos) {
		return Resultado{Fora, CausaExterno, "nenhum de " + alvos(e.Externos) + " respondeu"}
	}
	if len(e.DNSSistema) > 0 && nenhumOK(e.DNSSistema) {
		if algumOK(e.DNSExterno) {
			return Resultado{Fora, CausaDNSConfigurado, alvos(e.DNSSistema) + " não responde"}
		}
		return Resultado{Fora, CausaDNS, "nenhum DNS respondeu"}
	}
	if falhos := falhas(e.HTTP); len(falhos) > 0 {
		return Resultado{Degradado, CausaServico, strings.Join(falhos, ", ")}
	}
	if lat, perda, ok := mediaExternos(e.Externos); ok {
		if perda > l.Perda {
			return Resultado{Degradado, CausaPerda, fmt.Sprintf("%.0f%% de perda", perda*100)}
		}
		if lat > l.LatenciaMS {
			return Resultado{Degradado, CausaLatencia, fmt.Sprintf("%.0f ms em média", lat)}
		}
	}
	return Resultado{OK, CausaNenhuma, ""}
}

func nenhumOK(s []Sonda) bool { return !algumOK(s) }

func algumOK(s []Sonda) bool {
	for _, x := range s {
		if x.OK {
			return true
		}
	}
	return false
}

func falhas(s []Sonda) []string {
	var r []string
	for _, x := range s {
		if !x.OK {
			r = append(r, x.Alvo)
		}
	}
	return r
}

func alvos(s []Sonda) string {
	nomes := make([]string, len(s))
	for i, x := range s {
		nomes[i] = x.Alvo
	}
	return strings.Join(nomes, ", ")
}

// mediaExternos: latência média dos que responderam e perda média de todos.
func mediaExternos(s []Sonda) (lat, perda float64, ok bool) {
	var n int
	for _, x := range s {
		perda += x.Perda
		if x.OK {
			lat += x.LatenciaMS
			n++
		}
	}
	if n == 0 {
		return 0, 0, false
	}
	return lat / float64(n), perda / float64(len(s)), true
}
