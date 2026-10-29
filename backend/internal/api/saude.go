package api

import (
	"context"
	"net/http"
	"time"

	"netguard/internal/armazenamento"
	"netguard/internal/computador"
	"netguard/internal/saude"
)

type ConsultasSaude interface {
	ResumoRede(ctx context.Context, de, ate time.Time) (armazenamento.ResumoRede, error)
}

type FonteComputador interface {
	Resumo() (*computador.Resumo, bool)
}

// saudeDaRede junta o que foi medido e devolve o relatório com a nota e o que fazer.
func (s *servidor) saudeDaRede(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	agora := s.agora()
	semana, dia := agora.Add(-7*24*time.Hour), agora.Add(-24*time.Hour)
	aj := s.Ajustes.Atuais()
	e := saude.Entrada{Agora: agora, PlanoDown: aj.PlanoDownloadMbps, PlanoUp: aj.PlanoUploadMbps, DNSExterno: aj.DNSComparacao}

	falhou := func(err error) bool {
		if err != nil {
			s.falhaInterna(w, "relatório de saúde", err)
			return true
		}
		return false
	}
	disp, err := s.Consultas.Disponibilidade(ctx, semana, agora)
	if falhou(err) {
		return
	}
	e.Cobertura7dS = disp.SegundosCobertos
	if e.Quedas7d, err = s.Consultas.Incidentes(ctx, semana, agora, 500); falhou(err) {
		return
	}
	if e.Resumo24h, err = s.ConsultasSaude.ResumoRede(ctx, dia, agora); falhou(err) {
		return
	}
	if e.Resumo7d, err = s.ConsultasSaude.ResumoRede(ctx, semana, agora); falhou(err) {
		return
	}
	if e.PorHora7d, err = s.Consultas.Historico(ctx, semana.Truncate(time.Hour), agora, time.Hour); falhou(err) {
		return
	}
	if e.Testes7d, err = s.Consultas2B.TestesVelocidade(ctx, semana, agora); falhou(err) {
		return
	}
	if e.Wifi24h, err = s.Consultas2B.HistoricoWifi(ctx, dia.Truncate(time.Hour), agora, time.Hour); falhou(err) {
		return
	}
	if _, res, err := s.listaDispositivos(ctx); falhou(err) {
		return
	} else {
		e.Dispositivos = saude.Dispositivos{Total: res.Total, NaRede: res.NaRede, Novos: res.Novos, ProcuraAtiva: s.Descoberta.Estado().VarreduraAtiva}
	}
	if est, ok := s.Monitor.Estado(); ok {
		e.InterfaceTipo = est.Interface.Tipo
		if len(est.DNSSistema) > 0 {
			e.DNSSistema = est.DNSSistema[0].Alvo
		}
	}
	e.WifiAgora, _ = s.Wifi.Ultima()
	e.Vizinhos, _, _ = s.Vizinhanca.Ultima()
	e.Computador, _ = s.Computador.Resumo()

	escreverJSON(w, http.StatusOK, saude.Avaliar(e))
}

// computador devolve a leitura da máquina (para a tela mostrar os números ao vivo).
func (s *servidor) computador(w http.ResponseWriter, r *http.Request) {
	res, ok := s.Computador.Resumo()
	if !ok {
		escreverJSON(w, http.StatusOK, indisponivel("Aguardando a primeira leitura."))
		return
	}
	escreverJSON(w, http.StatusOK, map[string]any{"disponivel": true, "resumo": res})
}
