package armazenamento

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"netguard/internal/ajustes"
	"netguard/internal/auth"
	"netguard/internal/diagnostico"
	"netguard/internal/dispositivos"
	"netguard/internal/incidentes"
	"netguard/internal/velocidade"
	"netguard/internal/wifi"
)

// bancoDeTeste cria um schema próprio no database netguard_test e apaga no fim.
// Sem URL de teste configurada, o teste é pulado.
func bancoDeTeste(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("NETGUARD_BANCO_URL_TESTE")
	if url == "" {
		casa, _ := os.UserHomeDir()
		if b, err := os.ReadFile(filepath.Join(casa, ".config/netguard/banco_url_teste")); err == nil {
			url = strings.TrimSpace(string(b))
		}
	}
	if url == "" {
		t.Skip("sem NETGUARD_BANCO_URL_TESTE nem ~/.config/netguard/banco_url_teste")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	bruto := make([]byte, 4)
	rand.Read(bruto)
	schema := "teste_" + hex.EncodeToString(bruto)

	adm, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adm.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		adm.Close()
		t.Skipf("banco de teste indisponível: %v", err)
	}

	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := conectarComConfig(ctx, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		adm.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		adm.Close()
	})
	return pool
}

func TestMigrarIdempotente(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()

	feitas, err := Migrar(ctx, pool)
	if err != nil {
		t.Fatal(err)
	}
	if len(feitas) == 0 {
		t.Fatal("nenhuma migração aplicada num banco vazio")
	}
	feitas, err = Migrar(ctx, pool)
	if err != nil || len(feitas) != 0 {
		t.Fatalf("segunda vez deveria não aplicar nada: %v %v", feitas, err)
	}
	for _, tabela := range []string{"usuarios", "sessoes", "auditoria", "verificacoes"} {
		var existe bool
		pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", tabela).Scan(&existe)
		if !existe {
			t.Errorf("tabela %s não foi criada", tabela)
		}
	}
}

func TestUsuariosESessoes(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)

	// Usa o serviço de verdade em cima do banco: é o caminho que o login percorre.
	s := auth.NovoServico(r, time.Hour)
	if err := s.CriarUsuario(ctx, "dono", "senha-bem-comprida"); err != nil {
		t.Fatal(err)
	}
	if err := s.CriarUsuario(ctx, "dono", "senha-bem-comprida"); !errors.Is(err, auth.ErrJaExiste) {
		t.Fatalf("duplicado: %v", err)
	}
	if n, _ := r.QuantosUsuarios(ctx); n != 1 {
		t.Fatalf("usuários: %d", n)
	}

	_, token, _, err := s.Entrar(ctx, "dono", "senha-bem-comprida", "10.0.0.2", "teste")
	if err != nil {
		t.Fatal(err)
	}
	u, _, err := s.Validar(ctx, token)
	if err != nil || u.Nome != "dono" {
		t.Fatalf("validar: %v %+v", err, u)
	}
	var guardado []byte
	pool.QueryRow(ctx, "SELECT token_hash FROM sessoes").Scan(&guardado)
	if string(guardado) == token {
		t.Fatal("token guardado em texto puro")
	}

	if err := s.TrocarSenha(ctx, "dono", "outra-senha-comprida"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.Validar(ctx, token); !errors.Is(err, auth.ErrSessaoInvalida) {
		t.Fatal("sessão sobreviveu à troca de senha")
	}
	if err := s.TrocarSenha(ctx, "ninguem", "outra-senha-comprida"); !errors.Is(err, auth.ErrNaoEncontrado) {
		t.Fatalf("trocar senha de inexistente: %v", err)
	}

	id, _, _ := r.BuscarUsuario(ctx, "dono")
	r.CriarSessao(ctx, []byte("velha"), id, time.Now().Add(-time.Minute), "", "")
	if n, err := r.ApagarSessoesExpiradas(ctx, time.Now()); err != nil || n != 1 {
		t.Fatalf("limpeza: %d %v", n, err)
	}
}

func TestAuditoriaEVerificacoes(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)

	if err := r.Auditar(ctx, Auditoria{Usuario: "dono", Acao: "login", IP: "10.0.0.2", Detalhe: map[string]any{"x": 1}}); err != nil {
		t.Fatal(err)
	}
	lat, perda := 6.8, 0.0
	vs := []Verificacao{
		{Em: time.Now(), Tipo: "gateway", Alvo: "10.0.0.1", OK: true, LatenciaMS: &lat, Perda: &perda},
		{Em: time.Now(), Tipo: "interface", Alvo: "wlan0", OK: false, Erro: "sem rota"},
	}
	if err := r.SalvarVerificacoes(ctx, vs); err != nil {
		t.Fatal(err)
	}
	var n int
	pool.QueryRow(ctx, "SELECT count(*) FROM verificacoes").Scan(&n)
	if n != 2 {
		t.Fatalf("verificações gravadas: %d", n)
	}
	ruim := -1.0
	if err := r.SalvarVerificacoes(ctx, []Verificacao{{Em: time.Now(), Tipo: "x", Alvo: "y", Perda: &ruim}}); err == nil {
		t.Fatal("perda negativa deveria ser recusada pelo banco")
	}
}

func TestMonitoramento(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)
	t0 := time.Date(2026, 10, 29, 8, 0, 0, 0, time.UTC)

	// Monitor rodou 10 min (60 ciclos de 10 s), com 1 min fora no meio.
	id, err := r.IniciarExecucao(ctx, t0, "teste")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 60; i++ {
		em := t0.Add(time.Duration(i) * 10 * time.Second)
		estado, causa := "ok", "ok"
		if i >= 30 && i < 36 {
			estado, causa = "fora", "externo"
		}
		lat, perda := 7.0+float64(i%3), 0.0
		ext := 40.0
		c := Ciclo{Em: em, Estado: estado, Causa: causa, Verificacoes: []Verificacao{
			{Em: em, Tipo: "gateway", Alvo: "10.0.0.1", OK: true, LatenciaMS: &lat, Perda: &perda},
		}}
		if estado == "ok" {
			c.Verificacoes = append(c.Verificacoes, Verificacao{Em: em, Tipo: "externo", Alvo: "1.1.1.1", OK: true, LatenciaMS: &ext, Perda: &perda})
		}
		if err := r.SalvarCiclo(ctx, c); err != nil {
			t.Fatal(err)
		}
		if err := r.BaterExecucao(ctx, id, em); err != nil {
			t.Fatal(err)
		}
	}

	fim := t0.Add(6 * time.Minute)
	q := incidentes.Incidente{Inicio: t0.Add(5 * time.Minute), Causa: diagnostico.CausaExterno, Causas: []diagnostico.Causa{diagnostico.CausaExterno}}
	qid, err := r.SalvarIncidente(ctx, q)
	if err != nil {
		t.Fatal(err)
	}
	if aberto, _ := r.IncidenteAberto(ctx); aberto == nil || aberto.ID != qid {
		t.Fatalf("queda aberta: %+v", aberto)
	}
	if _, err := r.SalvarIncidente(ctx, incidentes.Incidente{Inicio: t0, Causa: "gateway"}); err == nil {
		t.Fatal("o banco deveria recusar duas quedas abertas")
	}
	q.ID, q.Fim = qid, &fim
	if _, err := r.SalvarIncidente(ctx, q); err != nil {
		t.Fatal(err)
	}
	if aberto, _ := r.IncidenteAberto(ctx); aberto != nil {
		t.Fatal("queda deveria estar fechada")
	}

	ultima, ok, err := r.UltimaExecucaoFim(ctx)
	if err != nil || !ok || !ultima.Equal(t0.Add(590*time.Second)) {
		t.Fatalf("última execução: %v %v %v", ultima, ok, err)
	}

	// Pergunta por 1 h: só 10 min tinham monitor; 6 de 60 ciclos fora.
	d, err := r.Disponibilidade(ctx, t0.Add(-50*time.Minute), t0.Add(10*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if d.CiclosTotal != 60 || d.CiclosNoAr != 54 || d.Quedas != 1 || d.SegundosFora != 60 {
		t.Fatalf("disponibilidade: %+v", d)
	}
	if d.SegundosCobertos != 590 {
		t.Fatalf("cobertura: %v s", d.SegundosCobertos)
	}

	pts, err := r.Historico(ctx, t0, t0.Add(10*time.Minute), time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 10 {
		t.Fatalf("pontos: %d", len(pts))
	}
	if pts[0].GatewayMS == nil || *pts[0].GatewayMS != 8 || pts[0].FracaoFora == nil || *pts[0].FracaoFora != 0 {
		t.Fatalf("primeiro ponto: gw=%v fora=%v", pts[0].GatewayMS, pts[0].FracaoFora)
	}
	if pts[5].InternetMS != nil || *pts[5].FracaoFora != 1 {
		t.Fatalf("minuto da queda: internet=%v fora=%v", pts[5].InternetMS, *pts[5].FracaoFora)
	}

	lista, err := r.Incidentes(ctx, t0, t0.Add(time.Hour), 10)
	if err != nil || len(lista) != 1 || lista[0].Causas[0] != diagnostico.CausaExterno {
		t.Fatalf("lista de quedas: %+v %v", lista, err)
	}

	if n, err := r.LimparAntigos(ctx, t0.AddDate(0, 0, 36)); err != nil || n != 114 {
		t.Fatalf("limpeza (114 verificações de 35+ dias): %d %v", n, err)
	}
}

func TestFase2B(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)

	// configurações: nada salvo → ok=false; salva, lê igual; salva de novo substitui
	if _, ok, err := r.LerAjustes(ctx); ok || err != nil {
		t.Fatalf("sem ajustes salvos: %v %v", ok, err)
	}
	a := ajustes.Padrao(10 * time.Second)
	a.URLs = []string{"https://exemplo.com"}
	if err := r.SalvarAjustes(ctx, a, "dono"); err != nil {
		t.Fatal(err)
	}
	a.IntervaloS = 30
	if err := r.SalvarAjustes(ctx, a, "dono"); err != nil {
		t.Fatal(err)
	}
	lido, ok, err := r.LerAjustes(ctx)
	if err != nil || !ok || lido.IntervaloS != 30 || lido.URLs[0] != "https://exemplo.com" || lido.Velocidade.VezesPorDia != 3 {
		t.Fatalf("ajustes lidos: %+v %v %v", lido, ok, err)
	}

	// velocidade: um teste completo e um que falhou no upload
	t0 := time.Now().Add(-time.Hour).Truncate(time.Second)
	d, u, l := 240.5, 85.2, 50.0
	if err := r.SalvarTesteVelocidade(ctx, velocidade.Resultado{Em: t0, Origem: "agendado", DownloadMbps: &d, UploadMbps: &u, LatenciaMS: &l,
		BytesBaixados: 25 << 20, BytesEnviados: 10 << 20, DuracaoS: 3.2, Servidor: "GIG", Interface: "wlan0"}); err != nil {
		t.Fatal(err)
	}
	if err := r.SalvarTesteVelocidade(ctx, velocidade.Resultado{Em: t0.Add(time.Minute), Origem: "manual", DownloadMbps: &d,
		BytesBaixados: 25 << 20, Erro: "upload: tempo esgotado"}); err != nil {
		t.Fatal(err)
	}
	lista, err := r.TestesVelocidade(ctx, t0.Add(-time.Minute), time.Now())
	if err != nil || len(lista) != 2 {
		t.Fatalf("testes: %v %v", lista, err)
	}
	if *lista[0].DownloadMbps != 240.5 || lista[0].Servidor != "GIG" || lista[1].UploadMbps != nil || lista[1].Erro == "" {
		t.Fatalf("testes lidos: %+v / %+v", lista[0], lista[1])
	}
	if n, _ := r.BytesTesteVelocidade(ctx, t0.Add(-time.Minute), time.Now()); n != 60<<20 {
		t.Fatalf("bytes gastos: %d", n)
	}
	if ult, _ := r.UltimoTesteVelocidade(ctx); ult == nil || ult.Origem != "manual" {
		t.Fatalf("último: %+v", ult)
	}
	if err := r.SalvarTesteVelocidade(ctx, velocidade.Resultado{Em: t0, Origem: "outro"}); err == nil {
		t.Fatal("origem inválida deveria ser recusada")
	}

	// Wi-Fi: 3 leituras no mesmo minuto viram um ponto
	w0 := time.Date(2026, 10, 29, 10, 0, 5, 0, time.UTC)
	for i, s := range []int{70, 80, 90} {
		if err := r.SalvarWifi(ctx, wifi.Amostra{Em: w0.Add(time.Duration(i) * 10 * time.Second), SSID: "casa", Sinal: s, Canal: 48, TaxaMbps: 1170}); err != nil {
			t.Fatal(err)
		}
	}
	pts, err := r.HistoricoWifi(ctx, w0.Add(-time.Hour), w0.Add(time.Hour), time.Minute)
	if err != nil || len(pts) != 1 || *pts[0].Sinal != 80 || *pts[0].SinalMin != 70 || *pts[0].TaxaMbps != 1170 {
		t.Fatalf("wifi: %+v %v", pts, err)
	}
	if err := r.SalvarWifi(ctx, wifi.Amostra{Em: w0, Sinal: 150}); err == nil {
		t.Fatal("sinal acima de 100 deveria ser recusado")
	}
}

func TestDispositivos(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)
	mac := func(s string) net.HardwareAddr { m, _ := net.ParseMAC(s); return m }
	ip := netip.MustParseAddr
	t0 := time.Date(2026, 10, 29, 18, 0, 0, 0, time.UTC)
	janela := 10 * time.Minute

	roteador := dispositivos.Observacao{MAC: mac("54:6c:ac:12:34:43"), IP: ip("10.0.0.1"), Fabricante: "Intelbras", Nome: "meuintelbras"}
	pc := dispositivos.Observacao{MAC: mac("14:bd:61:12:34:c4"), IP: ip("10.0.0.209"), EsteAparelho: true}
	celular := dispositivos.Observacao{MAC: mac("56:e2:c7:12:34:ac"), IP: ip("10.0.0.2"), Aleatorio: true, Nome: "Celular-Android"}

	// primeira coleta: retrato inicial, ninguém é "novo"
	if n, err := r.RegistrarObservacoes(ctx, t0, []dispositivos.Observacao{roteador, pc, celular}, janela, false); err != nil || n != 0 {
		t.Fatalf("primeira coleta: %d %v", n, err)
	}
	// 5 min depois: mesmos aparelhos, celular trocou de IP e o nome não veio desta vez
	celular2 := celular
	celular2.IP, celular2.Nome = ip("10.0.0.50"), ""
	if _, err := r.RegistrarObservacoes(ctx, t0.Add(5*time.Minute), []dispositivos.Observacao{roteador, pc, celular2}, janela, false); err != nil {
		t.Fatal(err)
	}
	// aparelho desconhecido chega
	visita := dispositivos.Observacao{MAC: mac("00:11:22:33:44:55"), IP: ip("10.0.0.77")}
	if n, _ := r.RegistrarObservacoes(ctx, t0.Add(6*time.Minute), []dispositivos.Observacao{roteador, visita}, janela, false); n != 1 {
		t.Fatalf("aparelho novo: %d", n)
	}
	// celular volta 1 h depois: aparição nova
	if _, err := r.RegistrarObservacoes(ctx, t0.Add(65*time.Minute), []dispositivos.Observacao{celular2}, janela, false); err != nil {
		t.Fatal(err)
	}

	// primeira procura ativa acha a TV que já estava em casa: retrato, sem aviso
	if feita, _ := r.PrimeiraVarreduraFeita(ctx); feita {
		t.Fatal("ainda não houve procura")
	}
	tv := dispositivos.Observacao{MAC: mac("f8:01:b4:12:34:b4"), IP: ip("10.0.0.133")}
	if n, _ := r.RegistrarObservacoes(ctx, t0.Add(66*time.Minute), []dispositivos.Observacao{tv}, janela, true); n != 0 {
		t.Fatalf("primeira procura não pode avisar: %d", n)
	}
	if err := r.MarcarPrimeiraVarredura(ctx); err != nil {
		t.Fatal(err)
	}
	if feita, _ := r.PrimeiraVarreduraFeita(ctx); !feita {
		t.Fatal("primeira procura não ficou marcada")
	}
	// a partir daí, procura acha aparelho de fora: avisa
	estranho := dispositivos.Observacao{MAC: mac("aa:bb:cc:00:00:01"), IP: ip("10.0.0.90")}
	if n, _ := r.RegistrarObservacoes(ctx, t0.Add(70*time.Minute), []dispositivos.Observacao{estranho}, janela, false); n != 1 {
		t.Fatalf("procura seguinte deveria avisar: %d", n)
	}

	lista, err := r.Dispositivos(ctx)
	if err != nil || len(lista) != 6 {
		t.Fatalf("lista: %d %v", len(lista), err)
	}
	por := map[string]Dispositivo{}
	for _, d := range lista {
		por[d.MAC] = d
	}
	cel := por["56:e2:c7:12:34:ac"]
	if cel.IP != "10.0.0.50" || cel.NomeDescoberto != "Celular-Android" || !cel.MACAleatorio || cel.AlertarNovo {
		t.Fatalf("celular: %+v", cel)
	}
	if !por["14:bd:61:12:34:c4"].EsteAparelho || por["14:bd:61:12:34:c4"].AlertarNovo {
		t.Fatal("este aparelho")
	}
	novo := por["00:11:22:33:44:55"]
	if !novo.AlertarNovo {
		t.Fatal("visita deveria pedir aviso")
	}
	ap, _ := r.Aparicoes(ctx, cel.ID, 10)
	if len(ap) != 2 || !ap[1].Fim.Equal(t0.Add(5*time.Minute)) || ap[0].IP != "10.0.0.50" {
		t.Fatalf("aparições do celular: %+v", ap)
	}

	apelido, nota, sim := "Celular da Ana", "", true
	d, err := r.AtualizarDispositivo(ctx, novo.ID, Mudanca{Apelido: &apelido, Confiavel: &sim})
	if err != nil || d.Apelido != "Celular da Ana" || !d.Confiavel || d.AlertarNovo {
		t.Fatalf("marcar confiável: %+v %v", d, err)
	}
	d, _ = r.AtualizarDispositivo(ctx, novo.ID, Mudanca{Apelido: &nota})
	if d.Apelido != "" || !d.Confiavel {
		t.Fatalf("apagar apelido sem mexer no resto: %+v", d)
	}
	if _, err := r.AtualizarDispositivo(ctx, 99999, Mudanca{VistoNovo: true}); !errors.Is(err, ErrNaoEncontrado) {
		t.Fatalf("inexistente: %v", err)
	}
	longo := strings.Repeat("x", 61)
	if _, err := r.AtualizarDispositivo(ctx, novo.ID, Mudanca{Apelido: &longo}); err == nil {
		t.Fatal("apelido longo demais deveria ser recusado pelo banco")
	}
	// movimentos: celular entrou às 18:00, saiu (último visto 18:05), voltou às 19:05
	mov, err := r.Movimentos(ctx, t0.Add(-time.Hour), t0.Add(2*time.Hour), janela, 50)
	if err != nil {
		t.Fatal(err)
	}
	var doCelular []string
	for _, m := range mov {
		if m.DispositivoID == cel.ID {
			doCelular = append(doCelular, m.Tipo+" "+m.Em.UTC().Format("15:04")+" "+m.IP)
		}
	}
	if strings.Join(doCelular, " | ") != "saiu 19:05 10.0.0.50 | entrou 19:05 10.0.0.50 | saiu 18:05 10.0.0.50 | entrou 18:00 10.0.0.50" {
		t.Fatalf("movimentos do celular: %v", doCelular)
	}
	// quem foi visto há menos de 10 min ainda não "saiu"
	recente, _ := r.Movimentos(ctx, t0, t0.Add(66*time.Minute), janela, 50)
	for _, m := range recente {
		if m.Tipo == "saiu" && m.Em.After(t0.Add(56*time.Minute)) {
			t.Fatalf("saída dentro da janela não pode aparecer: %+v", m)
		}
	}

	// presença: celular entrou 2 vezes (18:00–18:05 e 19:05, instantânea) → 5 min
	pres, err := r.Presencas(ctx, t0.Add(-time.Hour), t0.Add(2*time.Hour), janela)
	if err != nil {
		t.Fatal(err)
	}
	if p := pres[cel.ID]; p.Entradas != 2 || p.Segundos != 300 {
		t.Fatalf("presença do celular: %+v", p)
	}
	// janela cortando no meio: só conta o pedaço de dentro, e a entrada de antes não conta
	if p := mapaPresenca(t, r, t0.Add(3*time.Minute), t0.Add(30*time.Minute))[cel.ID]; p.Entradas != 0 || p.Segundos != 120 {
		t.Fatalf("presença cortada: %+v", p)
	}

	novos, _ := r.AparelhosNovos(ctx, t0.Add(-time.Hour), t0.Add(2*time.Hour))
	if len(novos) != 5 { // roteador, celular, visita, TV e o estranho (este aparelho fica de fora)
		t.Fatalf("aparelhos novos no período: %d", len(novos))
	}
}

func TestResumoRede(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)
	t0 := time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC)
	vazio, err := r.ResumoRede(ctx, t0, t0.Add(time.Hour))
	if err != nil || vazio.Amostras != 0 || vazio.LatenciaMediana != nil {
		t.Fatalf("sem dados: %+v %v", vazio, err)
	}
	f := func(x float64) *float64 { return &x }
	var vs []Verificacao
	for i, lat := range []float64{40, 50, 60, 500} { // um pico não puxa a mediana
		em := t0.Add(time.Duration(i) * time.Minute)
		vs = append(vs,
			Verificacao{Em: em, Tipo: "externo", Alvo: "1.1.1.1", OK: true, LatenciaMS: f(lat), Perda: f(0)},
			Verificacao{Em: em, Tipo: "dns_sistema", Alvo: "8.8.8.8", OK: i != 3, LatenciaMS: f(45)},
			Verificacao{Em: em, Tipo: "dns_externo", Alvo: "1.1.1.1", OK: true, LatenciaMS: f(25)})
	}
	vs = append(vs, Verificacao{Em: t0, Tipo: "externo", Alvo: "8.8.8.8", OK: false, Perda: f(1)})
	if err := r.SalvarVerificacoes(ctx, vs); err != nil {
		t.Fatal(err)
	}
	x, err := r.ResumoRede(ctx, t0, t0.Add(time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	if *x.LatenciaMediana != 55 || *x.PerdaMedia != 0.2 || x.Amostras != 5 {
		t.Fatalf("latência/perda: %v %v %d", *x.LatenciaMediana, *x.PerdaMedia, x.Amostras)
	}
	if *x.DNSSistemaMS != 45 || *x.DNSExternoMS != 25 || *x.DNSSistemaFalhas != 0.25 || *x.DNSExternoFalhas != 0 {
		t.Fatalf("dns: %v %v %v %v", *x.DNSSistemaMS, *x.DNSExternoMS, *x.DNSSistemaFalhas, *x.DNSExternoFalhas)
	}
}

func mapaPresenca(t *testing.T, r *Repositorio, de, ate time.Time) map[int64]Presenca {
	t.Helper()
	m, err := r.Presencas(context.Background(), de, ate, 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

// NetGuard parado (sessão encerrada): ninguém "sai" nem "entra" por isso; a
// parada vira uma linha só. Quem chega bem depois de religar entra de verdade.
func TestMovimentosComParada(t *testing.T) {
	pool := bancoDeTeste(t)
	ctx := context.Background()
	if _, err := Migrar(ctx, pool); err != nil {
		t.Fatal(err)
	}
	r := NovoRepositorio(pool)
	mac := func(s string) net.HardwareAddr { m, _ := net.ParseMAC(s); return m }
	ip := netip.MustParseAddr
	t0 := time.Date(2026, 10, 29, 10, 0, 0, 0, time.UTC)
	janela := 10 * time.Minute
	tv := dispositivos.Observacao{MAC: mac("f8:01:b4:12:34:b4"), IP: ip("10.0.0.133")}
	cel := dispositivos.Observacao{MAC: mac("56:e2:c7:12:34:ac"), IP: ip("10.0.0.2"), Aleatorio: true}
	visita := dispositivos.Observacao{MAC: mac("00:11:22:33:44:55"), IP: ip("10.0.0.77")}

	// 10:00–10:47 no ar (com atualização de 8 s às 10:20), parado até 12:00
	for _, e := range [][2]time.Duration{{0, 20 * time.Minute}, {20*time.Minute + 8*time.Second, 47 * time.Minute}, {120 * time.Minute, 180 * time.Minute}} {
		id, err := r.IniciarExecucao(ctx, t0.Add(e[0]), "teste")
		if err != nil {
			t.Fatal(err)
		}
		if err := r.BaterExecucao(ctx, id, t0.Add(e[1])); err != nil {
			t.Fatal(err)
		}
	}
	registrar := func(em time.Duration, obs ...dispositivos.Observacao) {
		t.Helper()
		if _, err := r.RegistrarObservacoes(ctx, t0.Add(em), obs, janela, false); err != nil {
			t.Fatal(err)
		}
	}
	for m := 0; m <= 45; m += 5 { // procura a cada 5 min
		registrar(time.Duration(m)*time.Minute, tv, cel)
	}
	registrar(121*time.Minute, tv)         // TV responde logo depois de religar: já estava
	registrar(122*time.Minute, tv, visita) // nunca esteve antes: entrou
	registrar(131*time.Minute, tv, visita)
	registrar(140*time.Minute, tv, visita, cel) // celular só 20 min depois: entrou de verdade

	mov, err := r.Movimentos(ctx, t0.Add(-time.Hour), t0.Add(145*time.Minute), janela, 50)
	if err != nil {
		t.Fatal(err)
	}
	var linhas []string
	for _, m := range mov {
		l := m.Tipo + " " + m.Em.UTC().Format("15:04")
		if m.Ate != nil {
			l += "-" + m.Ate.UTC().Format("15:04")
		}
		if m.IP != "" {
			l += " " + m.IP
		}
		linhas = append(linhas, l)
	}
	want := "entrou 12:20 10.0.0.2 | entrou 12:02 10.0.0.77 | desligado 10:47-12:00 | entrou 10:00 10.0.0.133 | entrou 10:00 10.0.0.2"
	if got := strings.Join(linhas, " | "); got != want {
		t.Fatalf("movimentos:\n got %s\nwant %s", got, want)
	}

	// presença: TV entrou 1 vez (a volta depois da parada não conta), celular 2
	pres := mapaPresenca(t, r, t0.Add(-time.Hour), t0.Add(145*time.Minute))
	lista, _ := r.Dispositivos(ctx)
	for _, d := range lista {
		want := map[string]int{"f8:01:b4:12:34:b4": 1, "56:e2:c7:12:34:ac": 2, "00:11:22:33:44:55": 1}[d.MAC]
		if pres[d.ID].Entradas != want {
			t.Fatalf("entradas de %s: %d, queria %d", d.MAC, pres[d.ID].Entradas, want)
		}
	}
}
