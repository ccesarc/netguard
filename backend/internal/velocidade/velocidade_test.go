package velocidade

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"netguard/internal/ajustes"
)

// servidorFalso imita o speed.cloudflare.com: /__down?bytes=N e /__up.
func servidorFalso(t *testing.T, recebido *int64) *httptest.Server {
	var mu sync.Mutex
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cf-Ray", "abc123-GIG")
		switch r.URL.Path {
		case "/__down":
			n, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
			w.Header().Set("Content-Length", strconv.Itoa(n))
			buf := make([]byte, 32<<10)
			for n > 0 {
				k := min(n, len(buf))
				w.Write(buf[:k])
				n -= k
			}
		case "/__up":
			n, _ := io.Copy(io.Discard, r.Body)
			mu.Lock()
			*recebido += n
			mu.Unlock()
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	return s
}

func TestMedir(t *testing.T) {
	var recebido int64
	s := servidorFalso(t, &recebido)
	m := NovoMedidor(s.URL, "teste")
	r := m.Medir(context.Background(), 8, 4)
	if r.Erro != "" {
		t.Fatalf("erro: %s", r.Erro)
	}
	if r.DownloadMbps == nil || *r.DownloadMbps <= 0 || r.UploadMbps == nil || *r.UploadMbps <= 0 || r.LatenciaMS == nil {
		t.Fatalf("medidas: %+v", r)
	}
	if r.BytesBaixados != 8<<20 || r.BytesEnviados != 4<<20 || recebido != 4<<20 {
		t.Fatalf("bytes: baixou %d, enviou %d, servidor recebeu %d", r.BytesBaixados, r.BytesEnviados, recebido)
	}
	if r.Servidor != "GIG" {
		t.Fatalf("servidor: %q", r.Servidor)
	}
}

func TestRespostaDuranteATransferencia(t *testing.T) {
	var recebido int64
	s := servidorFalso(t, &recebido)
	m := NovoMedidor(s.URL, "teste")
	m.Fluxos = 1
	lenta := func() { time.Sleep(900 * time.Millisecond) } // uma "transferência" que dura 0,9 s
	if v := m.sobCarga(context.Background(), lenta); v == nil || *v <= 0 {
		t.Fatalf("deveria medir a resposta durante a carga: %v", v)
	}
	if v := m.sobCarga(context.Background(), func() {}); v != nil {
		t.Fatalf("fase instantânea não dá para medir: %v", *v)
	}
}

func TestMedirServidorFora(t *testing.T) {
	var recebido int64
	s := servidorFalso(t, &recebido)
	s.Close()
	r := NovoMedidor(s.URL, "teste").Medir(context.Background(), 1, 1)
	if r.Erro == "" || r.DownloadMbps != nil || r.UploadMbps != nil {
		t.Fatalf("servidor fora deveria dar erro sem números: %+v", r)
	}
}

func TestProximoHorario(t *testing.T) {
	loc := time.FixedZone("BRT", -3*3600)
	h := func(d, hh, mm int) time.Time { return time.Date(2026, 10, d, hh, mm, 0, 0, loc) }
	casos := []struct {
		agora   time.Time
		vezes   int
		esperar time.Time
	}{
		{h(29, 10, 0), 3, h(29, 15, 0)},
		{h(29, 6, 59), 3, h(29, 7, 0)},
		{h(29, 7, 0), 3, h(29, 15, 0)},
		{h(29, 23, 30), 3, h(30, 7, 0)},
		{h(29, 2, 0), 3, h(29, 7, 0)},
		{h(29, 10, 0), 1, h(30, 7, 0)},
		{h(29, 10, 0), 12, h(29, 11, 0)},
	}
	for _, c := range casos {
		if p := ProximoHorario(c.agora, c.vezes); !p.Equal(c.esperar) {
			t.Errorf("agora %s, %d/dia: esperava %s, veio %s", c.agora.Format("02 15:04"), c.vezes, c.esperar.Format("02 15:04"), p.Format("02 15:04"))
		}
	}
}

type medicaoFalsa struct {
	libera chan struct{}
}

func (m medicaoFalsa) Medir(ctx context.Context, d, u int) Resultado {
	<-m.libera
	v := 100.0
	return Resultado{Em: time.Now(), DownloadMbps: &v, UploadMbps: &v}
}

type armazemFalso struct {
	mu sync.Mutex
	rs []Resultado
}

func (a *armazemFalso) SalvarTesteVelocidade(_ context.Context, r Resultado) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.rs = append(a.rs, r)
	return nil
}

func TestTestarAgora(t *testing.T) {
	libera := make(chan struct{})
	a := &armazemFalso{}
	g := NovoAgendador(medicaoFalsa{libera}, a,
		func() ajustes.Velocidade {
			return ajustes.Velocidade{Ativo: true, VezesPorDia: 3, MBDownload: 5, MBUpload: 2}
		},
		func() bool { return true }, func() string { return "wlan0" },
		slog.New(slog.NewTextHandler(io.Discard, nil)))

	if err := g.TestarAgora(); err != nil {
		t.Fatal(err)
	}
	if !g.Ocupado() || !g.Estado().Rodando {
		t.Fatal("deveria estar rodando")
	}
	if err := g.TestarAgora(); !errors.Is(err, ErrRodando) {
		t.Fatalf("segundo teste junto: %v", err)
	}
	close(libera)
	for i := 0; i < 100 && g.Ocupado(); i++ {
		time.Sleep(10 * time.Millisecond)
	}
	e := g.Estado()
	if e.Rodando || e.Ultimo == nil || e.Ultimo.Origem != "manual" || e.Ultimo.Interface != "wlan0" {
		t.Fatalf("estado depois: %+v", e)
	}
	if len(a.rs) != 1 {
		t.Fatal("resultado não gravado")
	}
	if err := g.TestarAgora(); !errors.Is(err, ErrCedo) {
		t.Fatalf("logo em seguida deveria pedir para esperar: %v", err)
	}
	if e.Proximo == nil {
		t.Fatal("com teste ativo deveria ter próximo horário")
	}
}

func TestProgressoDoTeste(t *testing.T) {
	var recebido int64
	s := servidorFalso(t, &recebido)
	m := NovoMedidor(s.URL, "teste")
	if m.Progresso() != nil {
		t.Fatal("fora de teste não tem progresso")
	}
	var fases []string
	var mu sync.Mutex
	pronto := make(chan struct{})
	go func() {
		for {
			select {
			case <-pronto:
				return
			default:
			}
			if p := m.Progresso(); p != nil {
				mu.Lock()
				if len(fases) == 0 || fases[len(fases)-1] != p.Fase {
					fases = append(fases, p.Fase)
				}
				if p.Total > 0 && p.Bytes > p.Total {
					t.Errorf("bytes passaram do total: %+v", p)
				}
				mu.Unlock()
			}
			time.Sleep(time.Millisecond)
		}
	}()
	m.Medir(context.Background(), 40, 20)
	close(pronto)
	mu.Lock()
	defer mu.Unlock()
	if strings.Join(fases, ",") != "resposta,download,upload" {
		t.Fatalf("fases vistas: %v", fases)
	}
	if m.Progresso() != nil {
		t.Fatal("depois do teste o progresso some")
	}
}
