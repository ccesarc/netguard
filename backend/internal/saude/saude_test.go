package saude

import (
	"strings"
	"testing"
	"time"

	"netguard/internal/armazenamento"
	"netguard/internal/computador"
	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

var agora = time.Date(2026, 11, 5, 15, 0, 0, 0, time.Local)

func f(x float64) *float64 { return &x }

// casa: a rede como medida em 2026-10-29, com uma semana de dados.
func casa() Entrada {
	var porHora []armazenamento.PontoHistorico
	for h := 0; h < 7*24; h++ {
		em := agora.Add(-time.Duration(h) * time.Hour).Truncate(time.Hour)
		lat := 48.0
		if em.Hour() >= 20 && em.Hour() <= 22 {
			lat = 130 // pico à noite
		}
		porHora = append(porHora, armazenamento.PontoHistorico{Em: em, InternetMS: f(lat)})
	}
	return Entrada{
		Agora:         agora,
		InterfaceTipo: "wifi",
		Cobertura7dS:  6 * 86400,
		Resumo24h:     armazenamento.ResumoRede{LatenciaMediana: f(48), PerdaMedia: f(0.001), Amostras: 8000},
		Resumo7d:      armazenamento.ResumoRede{DNSSistemaMS: f(45), DNSExternoMS: f(25), DNSSistemaFalhas: f(0), Amostras: 50000},
		PorHora7d:     porHora,
		Testes7d: []velocidade.Resultado{
			{DownloadMbps: f(273), UploadMbps: f(104), LatenciaMS: f(50), LatenciaDownloadMS: f(180), LatenciaUploadMS: f(90), Interface: "wlan0"},
			{DownloadMbps: f(265), UploadMbps: f(110), LatenciaMS: f(48), LatenciaDownloadMS: f(170), LatenciaUploadMS: f(85), Interface: "wlan0"},
		},
		PlanoDown: 350, PlanoUp: 150,
		WifiAgora: &wifi.Amostra{Sinal: 79},
		Wifi24h:   []armazenamento.PontoWifi{{Sinal: f(79), SinalMin: f(70)}, {Sinal: f(80), SinalMin: f(75)}},
		Vizinhos: &wifi.Vizinhanca{
			Redes: []wifi.Rede{
				{SSID: "MINHA_REDE_5G", Seguranca: "WPA2", Conectada: true, Propria: true, FreqMHz: 5240},
				{SSID: "MINHA_REDE", Seguranca: "WPA2", Propria: true, FreqMHz: 2412},
			},
			F5:     wifi.Faixa{Nome: "5 GHz", TuaRede: "MINHA_REDE_5G", CanalAtual: 48, Recomendado: 48, Vizinhos: 4},
			F24:    wifi.Faixa{Nome: "2,4 GHz", TuaRede: "MINHA_REDE", CanalAtual: 1, Recomendado: 1, Vizinhos: 20},
			Buscas: 6,
		},
		Dispositivos: Dispositivos{Total: 9, NaRede: 8, ProcuraAtiva: true},
		DNSSistema:   "8.8.8.8",
		DNSExterno:   "1.1.1.1",
		Computador: &computador.Resumo{
			Agora: computador.Leitura{MemTotalMB: 7808, MemDisponivelMB: 5524, SwapTotalMB: 7808, SwapUsadoMB: 420,
				PressaoMem: f(0), PressaoCPU: f(0.4), PressaoDisco: f(0.39), Carga5: 0.35, Nucleos: 4,
				Discos: []computador.Disco{{Ponto: "/", TotalGB: 443, LivreGB: 280, UsoPct: 37}}, TempCPU: f(56), TempSensor: "coretemp", LigadoS: 17280},
			TempMaxHora: f(61), MemMinHoraMB: 5000,
		},
	}
}

func porID(r Relatorio) map[string]Item {
	m := map[string]Item{}
	for _, it := range r.Itens {
		m[it.ID] = it
	}
	return m
}

func TestCasaComoEsta(t *testing.T) {
	r := Avaliar(casa())
	i := porID(r)
	esperado := map[string]Estado{
		"estabilidade": OK, "wifi_pc": OK, "latencia": OK, "perda": OK, "pico": Atencao,
		"velocidade": OK, "fila": Problema, "dns": Dica, "sinal": OK, "canal": OK,
		"faixa24": Dica, "aparelhos": OK, "seguranca": Dica,
		"memoria": OK, "cpu": OK, "disco": OK, "temperatura": OK, "ligado": OK,
	}
	for id, e := range esperado {
		if i[id].Estado != e {
			t.Errorf("%s: esperava %s, veio %s (%s · %s)", id, e, i[id].Estado, i[id].Resumo, i[id].Prova)
		}
	}
	if !strings.Contains(i["pico"].Resumo, "Entre 20h e 23h") {
		t.Errorf("pico: %s", i["pico"].Resumo)
	}
	if !strings.Contains(i["velocidade"].Prova, "77% / 71% do plano 350/150") {
		t.Errorf("velocidade: %s", i["velocidade"].Prova)
	}
	if !strings.Contains(i["fila"].Prova, "nota C") {
		t.Errorf("fila: %s", i["fila"].Prova)
	}
	if r.Nota == nil || *r.Nota < 70 || *r.Nota > 90 {
		t.Fatalf("nota: %v %s", r.Nota, r.Rotulo)
	}
	// o que fazer: primeiro o problema (fila), depois a atenção (pico), depois as dicas
	if len(r.Recomendacoes) < 3 || r.Recomendacoes[0].ID != "fila" || r.Recomendacoes[1].ID != "pico" {
		ids := []string{}
		for _, x := range r.Recomendacoes {
			ids = append(ids, x.ID)
		}
		t.Fatalf("ordem das recomendações: %v", ids)
	}
}

func TestSemDadosNaoInventa(t *testing.T) {
	r := Avaliar(Entrada{Agora: agora, InterfaceTipo: "wifi"})
	if r.Nota != nil || r.Rotulo != "Ainda juntando dados" {
		t.Fatalf("sem dados não pode ter nota: %v %s", r.Nota, r.Rotulo)
	}
	for _, it := range r.Itens {
		if it.Estado != SemDados {
			t.Errorf("%s deveria estar sem dados: %s", it.ID, it.Estado)
		}
	}
}

func TestPlanoNaoInformado(t *testing.T) {
	e := casa()
	e.PlanoDown, e.PlanoUp = 0, 0
	it := porID(Avaliar(e))["velocidade"]
	if it.Estado != SemDados || !strings.Contains(it.Recomendacao, "Configurações") || !strings.Contains(it.Resumo, "269") {
		t.Fatalf("sem plano: %+v", it)
	}
}

func TestVelocidadeNoCaboExigeMais(t *testing.T) {
	e := casa()
	for i := range e.Testes7d {
		e.Testes7d[i].Interface = "enp3s0f0"
	}
	it := porID(Avaliar(e))["velocidade"]
	if it.Estado != Atencao || !strings.Contains(it.Recomendacao, "provedor") {
		t.Fatalf("78%% no cabo é pouco: %+v", it)
	}
}

func TestQuedas(t *testing.T) {
	e := casa()
	fim := agora.Add(-2 * time.Hour)
	e.Quedas7d = []incidentes.Incidente{
		{Inicio: fim.Add(-40 * time.Second), Fim: &fim, Causa: diagnostico.CausaExterno},
		{Inicio: fim.Add(-time.Hour), Fim: &fim, Causa: diagnostico.CausaRedeLocal},
	}
	i := porID(Avaliar(e))
	if i["estabilidade"].Estado != Atencao || !strings.Contains(i["estabilidade"].Resumo, "1 queda") || !strings.Contains(i["estabilidade"].Recomendacao, "provedor") {
		t.Fatalf("estabilidade: %+v", i["estabilidade"])
	}
	if i["wifi_pc"].Estado != Atencao || !strings.Contains(i["wifi_pc"].Recomendacao, "cabo") {
		t.Fatalf("queda da placa conta separado: %+v", i["wifi_pc"])
	}
	for k := 0; k < 3; k++ {
		e.Quedas7d = append(e.Quedas7d, incidentes.Incidente{Inicio: fim.Add(-time.Minute), Fim: &fim, Causa: diagnostico.CausaGateway})
	}
	if it := porID(Avaliar(e))["estabilidade"]; it.Estado != Problema || !strings.Contains(it.Recomendacao, "roteador") {
		t.Fatalf("muitas quedas do roteador: %+v", it)
	}
}

func TestNoCaboSemItensDeWifi(t *testing.T) {
	e := casa()
	e.InterfaceTipo = "cabo"
	i := porID(Avaliar(e))
	if _, tem := i["sinal"]; tem {
		t.Fatal("no cabo não avalia o sinal do Wi-Fi do PC")
	}
	if _, tem := i["wifi_pc"]; tem {
		t.Fatal("no cabo não avalia quedas do Wi-Fi do PC")
	}
}

func TestPicoPrecisaDeDoisDias(t *testing.T) {
	e := casa()
	e.PorHora7d = e.PorHora7d[:20] // menos de 1 dia
	if it := porID(Avaliar(e))["pico"]; it.Estado != SemDados {
		t.Fatalf("com menos de 2 dias: %+v", it)
	}
}

func TestSegurancaAberta(t *testing.T) {
	e := casa()
	e.Vizinhos.Redes[1].Seguranca = ""
	if it := porID(Avaliar(e))["seguranca"]; it.Estado != Problema {
		t.Fatalf("rede aberta: %+v", it)
	}
}

func TestCanalParaTrocar(t *testing.T) {
	e := casa()
	e.Vizinhos.F24.Mudar, e.Vizinhos.F24.Recomendado = true, 11
	it := porID(Avaliar(e))["canal"]
	if it.Estado != Atencao || !strings.Contains(it.Recomendacao, "2,4 GHz: do 1 para o 11") {
		t.Fatalf("canal: %+v", it)
	}
}

func TestFilaBoa(t *testing.T) {
	e := casa()
	for i := range e.Testes7d {
		e.Testes7d[i].LatenciaDownloadMS = f(60)
		e.Testes7d[i].LatenciaUploadMS = f(55)
	}
	if it := porID(Avaliar(e))["fila"]; it.Estado != OK || !strings.Contains(it.Resumo, "nota A") {
		t.Fatalf("fila boa: %+v", it)
	}
}

func TestComputadorApertado(t *testing.T) {
	e := casa()
	c := e.Computador
	c.Agora.MemDisponivelMB, c.Agora.PressaoMem = 300, f(12)
	c.Agora.Discos[0].UsoPct = 92
	c.TempMaxHora = f(88)
	c.Agora.Carga5 = 7
	c.Agora.LigadoS = 40 * 86400
	i := porID(Avaliar(e))
	for id, esperado := range map[string]Estado{"memoria": Problema, "disco": Problema, "temperatura": Problema, "cpu": Problema, "ligado": Dica} {
		if i[id].Estado != esperado {
			t.Errorf("%s: esperava %s, veio %s (%s)", id, esperado, i[id].Estado, i[id].Prova)
		}
	}
	if !strings.Contains(i["temperatura"].Resumo, "88") || !strings.Contains(i["memoria"].Recomendacao, "painel") {
		t.Fatalf("textos: %s | %s", i["temperatura"].Resumo, i["memoria"].Recomendacao)
	}
}

func TestComputadorSemLeitura(t *testing.T) {
	e := casa()
	e.Computador = nil
	i := porID(Avaliar(e))
	if i["memoria"].Estado != SemDados || i["temperatura"].Estado != SemDados {
		t.Fatalf("sem leitura: %+v", i["memoria"])
	}
	if _, tem := i["ligado"]; tem {
		t.Fatal("sem leitura não mostra tempo ligado")
	}
}

func TestVirgula(t *testing.T) {
	casos := map[string]string{
		"5.3 de 7.6 GB livres":         "5,3 de 7,6 GB livres",
		"configurado (8.8.8.8) 45 ms":  "configurado (8.8.8.8) 45 ms",
		"perda 0.25% · 2.7×":           "perda 0,25% · 2,7×",
		"1.1.1.1 respondeu em 12.5 ms": "1.1.1.1 respondeu em 12,5 ms",
		"":                             "",
	}
	for de, para := range casos {
		if got := virgula(de); got != para {
			t.Errorf("%q: esperava %q, veio %q", de, para, got)
		}
	}
}

func TestSemOBasicoNaoTemNota(t *testing.T) {
	e := casa()
	e.Cobertura7dS = 3600 // estabilidade sem dados, mesmo com o resto medido
	if r := Avaliar(e); r.Nota != nil {
		t.Fatalf("sem a estabilidade medida não pode dar nota: %v", *r.Nota)
	}
}
