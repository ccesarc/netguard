package wifi

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Busca real deste PC em 2026-10-29 (BSSIDs dos vizinhos trocados por fictícios).
const buscaExemplo = ` :54\:6C\:AC\:12\:34\:44:MINHA_REDE:1:2412 MHz:95:WPA2
 :AA\:00\:00\:00\:00\:01:Vizinho1:6:2437 MHz:90:WPA2
*:54\:6C\:AC\:12\:34\:45:MINHA_REDE_5G:48:5240 MHz:79:WPA2
 :AA\:00\:00\:00\:00\:02:Vizinho1_5G:149:5745 MHz:72:WPA2
 :AA\:00\:00\:00\:00\:03:ESCRITORIO:11:2462 MHz:42:WPA2
 :AA\:00\:00\:00\:00\:04:CAM_CARRO_5A27:6:2437 MHz:40:WPA1 WPA2
 :AA\:00\:00\:00\:00\:05:Rede_B3:11:2462 MHz:40:WPA3
 :AA\:00\:00\:00\:00\:06:REDE_C:11:2462 MHz:40:WPA2
 :AA\:00\:00\:00\:00\:07:Vizinho2:1:2412 MHz:37:WPA2
 :AA\:00\:00\:00\:00\:08:fibra-1234567:8:2447 MHz:35:WPA2
 :AA\:00\:00\:00\:00\:09:VIZINHO3:10:2457 MHz:35:WPA2
 :AA\:00\:00\:00\:00\:0A:5G_Fibra_Vizinho4:157:5785 MHz:27:WPA1 WPA2
 :AA\:00\:00\:00\:00\:0B:5G Vizinho5 Net.Com:161:5805 MHz:22:WPA2
 :AA\:00\:00\:00\:00\:0C:Vizinho6:153:5765 MHz:20:WPA1 WPA2
`

func TestAnalisarLista(t *testing.T) {
	redes := analisarLista(buscaExemplo)
	if len(redes) != 14 || redes[0].SSID != "MINHA_REDE" || redes[0].Sinal != 95 {
		t.Fatalf("lista: %d, primeira %+v", len(redes), redes[0])
	}
	proprias := 0
	for _, r := range redes {
		if r.Propria {
			proprias++
		}
	}
	if proprias != 2 {
		t.Fatalf("as duas faixas do próprio roteador deveriam ser reconhecidas: %d", proprias)
	}
}

func TestAnaliseDaCasa(t *testing.T) {
	redes := analisarLista(buscaExemplo)

	f24 := Analisar24(redes)
	if f24.CanalAtual != 1 || f24.TuaRede != "MINHA_REDE" || f24.Recomendado != 1 || f24.Mudar {
		t.Fatalf("2,4 GHz: %+v", f24)
	}
	if f24.Texto != "Canal 1 está bom: só Vizinho2 (37%), fraco, divide o ar. Não mexa." {
		t.Fatalf("texto 2,4: %s", f24.Texto)
	}
	if f24.Vizinhos != 8 || len(f24.Canais) != 13 || f24.Canais[5].MaisForte != 90 {
		t.Fatalf("ocupação 2,4: vizinhos=%d canal6=%+v", f24.Vizinhos, f24.Canais[5])
	}

	f5 := Analisar5(redes)
	if f5.CanalAtual != 48 || f5.Recomendado != 48 || f5.Mudar || !strings.Contains(f5.Texto, "livre") {
		t.Fatalf("5 GHz: %+v", f5)
	}
	if f5.Vizinhos != 4 {
		t.Fatalf("vizinhos 5 GHz: %d", f5.Vizinhos)
	}
}

func TestRecomendaTrocarQuandoVale(t *testing.T) {
	// teu roteador no 6, junto com o vizinho forte; 1 e 11 vazios
	redes := []Rede{
		{BSSID: "54:6C:AC:12:34:44", SSID: "casa", Canal: 6, FreqMHz: 2437, Sinal: 95, Conectada: true},
		{BSSID: "AA:00:00:00:00:01", SSID: "vizinho", Canal: 6, FreqMHz: 2437, Sinal: 85},
	}
	f := Analisar24(redes)
	if !f.Mudar || f.Recomendado == 6 || !strings.Contains(f.Texto, "vizinho (85%)") {
		t.Fatalf("deveria recomendar trocar: %+v", f)
	}
	// 5 GHz: dividindo o bloco 149–161 com vizinho forte; 36–48 livre
	redes5 := []Rede{
		{BSSID: "54:6C:AC:12:34:45", SSID: "casa5", Canal: 153, FreqMHz: 5765, Sinal: 80, Conectada: true},
		{BSSID: "AA:00:00:00:00:02", SSID: "viz5", Canal: 149, FreqMHz: 5745, Sinal: 75},
	}
	f5 := Analisar5(redes5)
	if !f5.Mudar || f5.Recomendado != 36 {
		t.Fatalf("5 GHz deveria recomendar o bloco 36–48: %+v", f5)
	}
}

func TestDiferencaPequenaNaoMuda(t *testing.T) {
	redes := []Rede{
		{BSSID: "54:6C:AC:12:34:44", SSID: "casa", Canal: 1, FreqMHz: 2412, Sinal: 95, Conectada: true},
		{BSSID: "AA:00:00:00:00:01", SSID: "fraco", Canal: 1, FreqMHz: 2412, Sinal: 30},
	}
	if f := Analisar24(redes); f.Mudar {
		t.Fatalf("vizinho fraco não justifica trocar: %+v", f)
	}
}

type buscadorFalso struct {
	redes []Rede
	err   error
}

func (b buscadorFalso) Buscar(context.Context, string) ([]Rede, error) { return b.redes, b.err }

func TestVigia(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	wifi := func() (string, string) { return "wlan0", "wifi" }
	nunca := func() bool { return false }

	v := NovoVigia(buscadorFalso{redes: analisarLista(buscaExemplo)}, wifi, nunca, log)
	v.repetir = false
	if err := v.BuscarAgora(context.Background()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		if _, _, b := v.Ultima(); !b {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	viz, _, _ := v.Ultima()
	if viz == nil || len(viz.Redes) != 14 || viz.F5.CanalAtual != 48 {
		t.Fatalf("vizinhança: %+v", viz)
	}
	if err := v.BuscarAgora(context.Background()); !errors.Is(err, ErrBuscaCedo) {
		t.Fatalf("logo depois: %v", err)
	}

	v = NovoVigia(buscadorFalso{err: errors.New("o sistema não autorizou")}, wifi, nunca, log)
	v.buscar(context.Background())
	if viz, motivo, _ := v.Ultima(); viz != nil || !strings.Contains(motivo, "não autorizou") {
		t.Fatalf("erro: %v %q", viz, motivo)
	}

	v = NovoVigia(buscadorFalso{}, func() (string, string) { return "eth0", "cabo" }, nunca, log)
	v.buscar(context.Background())
	if _, motivo, _ := v.Ultima(); !strings.Contains(motivo, "não está usando Wi-Fi") {
		t.Fatalf("no cabo: %q", motivo)
	}
}

func TestComparacaoExplica(t *testing.T) {
	redes := analisarLista(buscaExemplo)
	c := Analisar24(redes).Comparacao
	if len(c) != 3 {
		t.Fatalf("2,4: %+v", c)
	}
	// 1 (pouca) < 11 (três fracas, média) < 6 (uma forte, muita): mais redes no 11, mas melhor que o 6
	if c[0].Canal != 1 || c[1].Canal != 11 || c[2].Canal != 6 {
		t.Fatalf("ordem: %d %d %d", c[0].Canal, c[1].Canal, c[2].Canal)
	}
	if !c[0].Melhor || !c[0].Atual || c[0].Disputa != "pouca" || c[0].Motivo != "1 rede: Vizinho2 37% (fraca)" {
		t.Fatalf("canal 1: %+v", c[0])
	}
	if c[1].Disputa != "média" || !strings.HasPrefix(c[1].Motivo, "3 redes: ESCRITORIO 42% (média), Rede_B3 40% (média), REDE_C 40% (média)") || !strings.Contains(c[1].Motivo, "canal vizinho") {
		t.Fatalf("canal 11: %+v", c[1])
	}
	if c[2].Disputa != "muita" || !strings.HasPrefix(c[2].Motivo, "2 redes: Vizinho1 90% (forte), CAM_CARRO_5A27 40% (média)") {
		t.Fatalf("canal 6: %+v", c[2])
	}

	c5 := Analisar5(redes).Comparacao
	if len(c5) != 2 || c5[0].Rotulo != "canais 36–48" || c5[0].Motivo != "livre de vizinhos" || !c5[0].Atual || c5[0].Canal != 48 {
		t.Fatalf("5 GHz: %+v", c5)
	}
	if !strings.HasPrefix(c5[1].Motivo, "4 redes: Vizinho1_5G 72% (forte), 5G_Fibra_Vizinho4 27% (fraca), 5G Vizinho5 Net.Com 22% (fraca), +1") {
		t.Fatalf("bloco 149–161: %+v", c5[1])
	}
}

func TestLembraOTeuRoteador(t *testing.T) {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	completa := analisarLista(buscaExemplo)
	// segunda busca: o rádio 2,4 GHz do teu roteador não respondeu
	var sem24 []Rede
	for _, r := range completa {
		if r.SSID != "MINHA_REDE" {
			sem24 = append(sem24, r)
		}
	}
	b := &buscadorTrocavel{redes: completa}
	v := NovoVigia(b, func() (string, string) { return "wlan0", "wifi" }, func() bool { return false }, log)
	v.repetir = false
	v.buscar(context.Background())
	b.redes = sem24
	v.buscar(context.Background())
	viz, _, _ := v.Ultima()
	if viz.F24.CanalAtual != 1 || viz.F24.TuaRede != "MINHA_REDE" {
		t.Fatalf("deveria lembrar do rádio 2,4 GHz: %+v", viz.F24)
	}
}

type buscadorTrocavel struct{ redes []Rede }

func (b *buscadorTrocavel) Buscar(context.Context, string) ([]Rede, error) { return b.redes, nil }

func TestMediaDasBuscasEstabiliza(t *testing.T) {
	casa := Rede{BSSID: "54:6C:AC:12:34:44", SSID: "casa", Canal: 1, FreqMHz: 2412, Sinal: 95, Conectada: true}
	sempre := Rede{BSSID: "AA:00:00:00:00:01", SSID: "fraco", Canal: 1, FreqMHz: 2412, Sinal: 30}
	passageira := Rede{BSSID: "AA:00:00:00:00:02", SSID: "carro-do-vizinho", Canal: 1, FreqMHz: 2412, Sinal: 90}
	buscas := [][]Rede{{casa, sempre}, {casa, sempre}, {casa, sempre, passageira}, {casa, sempre}, {casa, sempre}, {casa, sempre}}
	m := mesclar(buscas)
	por := map[string]int{}
	for _, r := range m {
		por[r.SSID] = r.Sinal
	}
	if por["fraco"] != 30 || por["carro-do-vizinho"] != 15 {
		t.Fatalf("média: %v", por)
	}
	// numa busca só, a passageira forte manda trocar; na média, não
	if !Analisar24(buscas[2]).Mudar {
		t.Fatal("a busca isolada deveria mandar trocar (é o que a média evita)")
	}
	if Analisar24(m).Mudar {
		t.Fatal("a média não deveria mandar trocar por causa de uma rede que apareceu uma vez")
	}
}

func TestTextoCurto(t *testing.T) {
	redes := []Rede{{BSSID: "54:6C:AC:12:34:44", SSID: "casa", Canal: 1, FreqMHz: 2412, Sinal: 95, Conectada: true}}
	for i, s := range []int{50, 45, 44, 43, 42} {
		redes = append(redes, Rede{BSSID: "AA:00:00:00:00:0" + strconv.Itoa(i), SSID: "", Canal: 1, FreqMHz: 2412, Sinal: s})
	}
	f := Analisar24(redes)
	if !strings.Contains(f.Texto, "rede oculta (50%)") || !strings.Contains(f.Texto, "mais 2") || strings.Contains(f.Texto, " (42%)") {
		t.Fatalf("texto: %s", f.Texto)
	}
}

func TestComparacaoTrazAsRedesSeparadas(t *testing.T) {
	c := Analisar24(analisarLista(buscaExemplo)).Comparacao
	seis := c[2]
	if len(seis.Redes) != 2 || seis.Redes[0] != (Vizinho{Nome: "Vizinho1", Sinal: 90, Nivel: "forte", Canal: 6}) || seis.Redes[1].Nivel != "média" {
		t.Fatalf("redes do canal 6: %+v", seis.Redes)
	}
	if len(seis.Perto) != 2 || seis.Perto[0].Canal != 8 {
		t.Fatalf("sobrepostas ao 6: %+v", seis.Perto)
	}
}

func TestReunirBuscas(t *testing.T) {
	casa := Rede{BSSID: "54:6C:AC:12:34:45", SSID: "casa5", Canal: 48, FreqMHz: 5240, Sinal: 79, Conectada: true}
	forte := Rede{BSSID: "AA:00:00:00:00:01", SSID: "Vizinho1", Canal: 6, FreqMHz: 2437, Sinal: 90}
	fraca := Rede{BSSID: "AA:00:00:00:00:02", SSID: "longe", Canal: 11, FreqMHz: 2462, Sinal: 20}
	fraca2 := fraca
	fraca2.Sinal = 30
	v := reunir([][]Rede{{casa, forte}, {casa, forte, fraca}, {casa, forte, fraca2}})
	if len(v) != 3 || v[0].SSID != "casa5" {
		t.Fatalf("a tua rede vem primeiro e nenhuma some: %+v", v)
	}
	if v[1].SSID != "Vizinho1" || v[1].Vezes != 3 || v[1].Buscas != 3 {
		t.Fatalf("Vizinho1: %+v", v[1])
	}
	if v[2].SinalMedio != 25 || v[2].Vezes != 2 || v[2].Sinal != 30 {
		t.Fatalf("rede fraca: média das vezes ouvida e a leitura mais recente: %+v", v[2])
	}
}

func TestJuntarDuasEscutas(t *testing.T) {
	a := []Rede{{BSSID: "A", SSID: "x", Sinal: 40}, {BSSID: "B", SSID: "y", Sinal: 50}}
	b := []Rede{{BSSID: "A", SSID: "x", Sinal: 55}, {BSSID: "C", SSID: "z", Sinal: 20}}
	j := juntarBuscas(a, b)
	if len(j) != 3 || j[0].BSSID != "A" || j[0].Sinal != 55 {
		t.Fatalf("junção: %+v", j)
	}
}
