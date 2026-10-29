package api

import (
	"net/http"
	"time"

	"netguard/internal/diagnostico"
	"netguard/internal/monitor"
)

// Indisponivel é o que vai no lugar de um dado que não existe: nunca zero nem número inventado.
type Indisponivel struct {
	Disponivel bool   `json:"disponivel"`
	Motivo     string `json:"motivo"`
}

func indisponivel(motivo string) Indisponivel { return Indisponivel{Motivo: motivo} }

type respInterface struct {
	Disponivel bool   `json:"disponivel"`
	Motivo     string `json:"motivo,omitempty"`
	Nome       string `json:"nome,omitempty"`
	Tipo       string `json:"tipo,omitempty"`
	Ativa      bool   `json:"ativa"`
	IPv4       string `json:"ipv4,omitempty"`
	Gateway    string `json:"gateway,omitempty"`
	OK         bool   `json:"ok"`
	Problema   string `json:"problema,omitempty"`
}

type respGateway struct {
	Disponivel bool   `json:"disponivel"`
	Motivo     string `json:"motivo,omitempty"`
	*monitor.MedidaPing
}

type respInternet struct {
	Disponivel bool                 `json:"disponivel"`
	Motivo     string               `json:"motivo,omitempty"`
	Estado     diagnostico.Estado   `json:"estado,omitempty"`
	Causa      *respCausa           `json:"causa,omitempty"`
	Detalhe    string               `json:"detalhe,omitempty"`
	LatenciaMS *float64             `json:"latencia_media_ms"`
	Perda      *float64             `json:"perda"`
	Externos   []monitor.MedidaPing `json:"externos,omitempty"`
	Servicos   []monitor.Medida     `json:"servicos,omitempty"`
}

type respDNS struct {
	Disponivel bool             `json:"disponivel"`
	Motivo     string           `json:"motivo,omitempty"`
	OK         bool             `json:"ok"`
	Sistema    []monitor.Medida `json:"sistema,omitempty"`
	Externo    []monitor.Medida `json:"externo,omitempty"`
}

type respVisaoGeral struct {
	MedidoEm     *time.Time    `json:"medido_em"`
	Interface    respInterface `json:"interface"`
	Gateway      respGateway   `json:"gateway"`
	Internet     respInternet  `json:"internet"`
	DNS          respDNS       `json:"dns"`
	Queda        *respQueda    `json:"queda"`
	Wifi         any           `json:"wifi"`
	Velocidade   any           `json:"velocidade"`
	Dispositivos any           `json:"dispositivos"`
	Bloqueio     Indisponivel  `json:"bloqueio"`
	Alertas      Indisponivel  `json:"alertas"`
}

func (s *servidor) visaoGeral(w http.ResponseWriter, r *http.Request) {
	resp := respVisaoGeral{
		Bloqueio: indisponivel("Precisa do Pi-hole, que vai rodar no Raspberry Pi."),
		Alertas:  indisponivel("Os alertas chegam na fase 5."),
	}
	if a, motivo := s.Wifi.Ultima(); a != nil {
		resp.Wifi = map[string]any{"disponivel": true, "amostra": a}
	} else {
		resp.Wifi = indisponivel(motivo)
	}
	if v := s.Velocidade.Estado(); v.Ultimo != nil {
		resp.Velocidade = map[string]any{"disponivel": true, "rodando": v.Rodando, "ultimo": v.Ultimo, "proximo": v.Proximo}
	} else {
		resp.Velocidade = map[string]any{"disponivel": false, "motivo": "Nenhum teste de velocidade ainda.", "rodando": v.Rodando, "proximo": v.Proximo}
	}

	if _, res, err := s.listaDispositivos(r.Context()); err != nil || res.Total == 0 {
		resp.Dispositivos = indisponivel("Nenhum aparelho observado ainda.")
	} else {
		resp.Dispositivos = map[string]any{"disponivel": true, "resumo": res, "varredura_ativa": s.Descoberta.Estado().VarreduraAtiva}
	}

	e, ok := s.Monitor.Estado()
	if !ok {
		const m = "Aguardando a primeira medição."
		resp.Interface = respInterface{Motivo: m}
		resp.Gateway = respGateway{Motivo: m}
		resp.Internet = respInternet{Motivo: m}
		resp.DNS = respDNS{Motivo: m}
		escreverJSON(w, http.StatusOK, resp)
		return
	}

	resp.MedidoEm = &e.MedidoEm
	i := e.Interface
	resp.Interface = respInterface{
		Disponivel: true, Nome: i.Nome, Tipo: i.Tipo, Ativa: i.Ativa, IPv4: i.IPv4,
		OK: e.ErroInterface == "", Problema: e.ErroInterface,
	}
	if i.TemRota() {
		resp.Interface.Gateway = i.Gateway.String()
	}
	if e.Gateway != nil {
		resp.Gateway = respGateway{Disponivel: true, MedidaPing: e.Gateway}
	} else {
		resp.Gateway = respGateway{Motivo: "Sem medir: este aparelho está sem rede."}
	}

	resp.Internet = respInternet{
		Disponivel: true,
		Estado:     e.Diagnostico.Estado,
		Causa:      &respCausa{e.Diagnostico.Causa, diagnostico.Texto(e.Diagnostico.Causa)},
		Detalhe:    e.Diagnostico.Detalhe,
		Externos:   e.Externos,
		Servicos:   e.HTTP,
	}
	resp.Internet.LatenciaMS, resp.Internet.Perda = mediaExternos(e.Externos)

	if len(e.DNSSistema)+len(e.DNSExterno) > 0 {
		resp.DNS = respDNS{Disponivel: true, OK: algumOK(e.DNSSistema), Sistema: e.DNSSistema, Externo: e.DNSExterno}
	} else {
		resp.DNS = respDNS{Motivo: "Sem medir: este aparelho está sem rede."}
	}

	if e.Queda != nil {
		q := montarQueda(*e.Queda, s.agora())
		resp.Queda = &q
	}
	escreverJSON(w, http.StatusOK, resp)
}

// mediaExternos: latência média de quem respondeu e perda média de todos; nil se nada respondeu.
func mediaExternos(ms []monitor.MedidaPing) (*float64, *float64) {
	if len(ms) == 0 {
		return nil, nil
	}
	var lat, perda float64
	var n int
	for _, m := range ms {
		perda += m.Perda
		if m.OK {
			lat += m.MediaMS
			n++
		}
	}
	perda /= float64(len(ms))
	if n == 0 {
		return nil, &perda
	}
	lat /= float64(n)
	return arred(&lat, 1), arred(&perda, 3)
}

func algumOK(ms []monitor.Medida) bool {
	for _, m := range ms {
		if m.OK {
			return true
		}
	}
	return false
}
