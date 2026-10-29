// Package ajustes guarda o que o dono muda pela tela de Configurações:
// destinos testados, intervalos, limites e o teste de velocidade.
// Tudo é validado aqui antes de chegar ao monitor.
package ajustes

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type Velocidade struct {
	Ativo       bool `json:"ativo"`
	VezesPorDia int  `json:"vezes_por_dia"`
	MBDownload  int  `json:"mb_download"`
	MBUpload    int  `json:"mb_upload"`
}

type Ajustes struct {
	IntervaloS       int        `json:"intervalo_s"`
	Externos         []string   `json:"externos"`
	DNSComparacao    string     `json:"dns_comparacao"`
	URLs             []string   `json:"urls"`
	LimiteLatenciaMS float64    `json:"limite_latencia_ms"`
	LimitePerda      float64    `json:"limite_perda"` // 0 a 1
	Velocidade       Velocidade `json:"velocidade"`
	// Varredura ativa: um ping em cada endereço da rede a cada 5 min para achar
	// os aparelhos. Desligada de fábrica: só liga com o dono autorizando.
	VarreduraAtiva bool `json:"varredura_ativa"`
	// Plano contratado (Mbps, no cabo). 0 = não informado.
	PlanoDownloadMbps int `json:"plano_download_mbps"`
	PlanoUploadMbps   int `json:"plano_upload_mbps"`
}

// Padrao são os valores de fábrica. intervalo vem do NETGUARD_INTERVALO.
func Padrao(intervalo time.Duration) Ajustes {
	return Ajustes{
		IntervaloS:       int(intervalo.Seconds()),
		Externos:         []string{"1.1.1.1", "8.8.8.8", "9.9.9.9"},
		DNSComparacao:    "1.1.1.1",
		URLs:             []string{"https://www.google.com/generate_204", "https://www.cloudflare.com/cdn-cgi/trace"},
		LimiteLatenciaMS: 150,
		LimitePerda:      0.1,
		Velocidade:       Velocidade{Ativo: true, VezesPorDia: 3, MBDownload: 25, MBUpload: 10},
	}
}

// Validar devolve todos os problemas de uma vez, em português.
func (a Ajustes) Validar() error {
	var erros []error
	ruim := func(f string, args ...any) { erros = append(erros, fmt.Errorf(f, args...)) }

	if a.IntervaloS < 5 || a.IntervaloS > 300 {
		ruim("intervalo: de 5 a 300 segundos")
	}
	if len(a.Externos) < 1 || len(a.Externos) > 5 {
		ruim("endereços externos: de 1 a 5")
	}
	vistos := map[string]bool{}
	for _, e := range a.Externos {
		ip, err := netip.ParseAddr(e)
		switch {
		case err != nil || !ip.Is4():
			ruim("endereço externo inválido: %q (use um IPv4, ex.: 1.1.1.1)", e)
		case ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() || ip.IsMulticast():
			ruim("endereço externo %s é da rede local: use um endereço da internet", e)
		case vistos[e]:
			ruim("endereço externo repetido: %s", e)
		}
		vistos[e] = true
	}
	if ip, err := netip.ParseAddr(a.DNSComparacao); err != nil || !ip.Is4() {
		ruim("DNS de comparação inválido: %q", a.DNSComparacao)
	}
	if len(a.URLs) > 5 {
		ruim("sites: no máximo 5")
	}
	for _, u := range a.URLs {
		if err := validarURL(u); err != nil {
			ruim("site %q: %v", u, err)
		}
	}
	if a.LimiteLatenciaMS < 20 || a.LimiteLatenciaMS > 2000 {
		ruim("limite de latência: de 20 a 2000 ms")
	}
	if a.LimitePerda < 0.01 || a.LimitePerda > 0.5 {
		ruim("limite de perda: de 1%% a 50%%")
	}
	v := a.Velocidade
	if v.VezesPorDia < 1 || v.VezesPorDia > 12 {
		ruim("teste de velocidade: de 1 a 12 vezes por dia")
	}
	if v.MBDownload < 5 || v.MBDownload > 200 {
		ruim("download do teste: de 5 a 200 MB")
	}
	if v.MBUpload < 2 || v.MBUpload > 100 {
		ruim("upload do teste: de 2 a 100 MB")
	}
	if a.PlanoDownloadMbps < 0 || a.PlanoDownloadMbps > 10000 || a.PlanoUploadMbps < 0 || a.PlanoUploadMbps > 10000 {
		ruim("plano contratado: de 0 (não sei) a 10000 Mbps")
	}
	return errors.Join(erros...)
}

func validarURL(s string) error {
	if len(s) > 200 {
		return errors.New("longo demais")
	}
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return errors.New("endereço inválido")
	}
	if u.Scheme != "https" && u.Scheme != "http" {
		return errors.New("use http:// ou https://")
	}
	if u.User != nil {
		return errors.New("não coloque usuário ou senha no endereço")
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return errors.New("tem espaço")
	}
	return nil
}

// ConsumoMensalMB estima quanto o teste de velocidade gasta por mês (30 dias).
func (a Ajustes) ConsumoMensalMB() int {
	v := a.Velocidade
	if !v.Ativo {
		return 0
	}
	return v.VezesPorDia * (v.MBDownload + v.MBUpload) * 30
}
