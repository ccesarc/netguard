// Package velocidade mede download e upload contra o servidor de teste da
// Cloudflare (o mesmo do speed.cloudflare.com) e agenda os testes do dia.
//
// Mede "até este aparelho": pelo Wi-Fi, o número inclui o Wi-Fi.
package velocidade

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Resultado struct {
	Em           time.Time `json:"em"`
	Origem       string    `json:"origem"` // agendado ou manual
	DownloadMbps *float64  `json:"download_mbps"`
	UploadMbps   *float64  `json:"upload_mbps"`
	LatenciaMS   *float64  `json:"latencia_ms"`
	// Resposta da internet enquanto o teste baixa/envia. A diferença para a
	// LatenciaMS mostra a "fila cheia" (bufferbloat): a internet trava quando alguém usa muito.
	LatenciaDownloadMS *float64 `json:"latencia_download_ms"`
	LatenciaUploadMS   *float64 `json:"latencia_upload_ms"`
	BytesBaixados      int64    `json:"bytes_baixados"`
	BytesEnviados      int64    `json:"bytes_enviados"`
	DuracaoS           float64  `json:"duracao_s"`
	Servidor           string   `json:"servidor,omitempty"` // código do datacenter (ex.: GIG = Rio)
	Interface          string   `json:"interface,omitempty"`
	Erro               string   `json:"erro,omitempty"`
}

type Medidor struct {
	Base    string // https://speed.cloudflare.com
	Fluxos  int    // conexões em paralelo no download (upload usa metade)
	Agente  string
	cliente *http.Client
	sonda   *http.Client // conexão à parte para medir a resposta durante o teste

	// progresso do teste em andamento (lido pela tela)
	mu         sync.Mutex
	fase       string
	faseInicio time.Time
	faseTotal  int64
	bytes      atomic.Int64
}

// ProgressoTeste é o que a tela mostra durante o teste.
type ProgressoTeste struct {
	Fase      string  `json:"fase"` // resposta, download, upload
	Bytes     int64   `json:"bytes"`
	Total     int64   `json:"total"`
	MbpsAgora float64 `json:"mbps_agora"`
}

func (m *Medidor) iniciarFase(fase string, total int64) {
	m.mu.Lock()
	m.fase, m.faseInicio, m.faseTotal = fase, time.Now(), total
	m.mu.Unlock()
	m.bytes.Store(0)
}

// Progresso devolve a fase atual e a velocidade até agora; nil fora de teste.
func (m *Medidor) Progresso() *ProgressoTeste {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fase == "" {
		return nil
	}
	p := &ProgressoTeste{Fase: m.fase, Bytes: m.bytes.Load(), Total: m.faseTotal}
	if d := time.Since(m.faseInicio); d > 300*time.Millisecond && p.Bytes > 0 {
		p.MbpsAgora = mbps(p.Bytes, d)
	}
	return p
}

func NovoMedidor(base, agente string) *Medidor {
	return &Medidor{
		Base:   strings.TrimRight(base, "/"),
		Fluxos: 4,
		Agente: agente,
		sonda: &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{
			TLSNextProto:        map[string]func(string, *tls.Conn) http.RoundTripper{},
			MaxIdleConnsPerHost: 1,
		}},
		cliente: &http.Client{Transport: &http.Transport{
			// HTTP/1.1: uma conexão TCP por fluxo, como os medidores conhecidos.
			TLSNextProto:          map[string]func(string, *tls.Conn) http.RoundTripper{},
			MaxIdleConnsPerHost:   8,
			ResponseHeaderTimeout: 10 * time.Second,
			DisableCompression:    true,
		}},
	}
}

// Medir roda latência, download e upload, nessa ordem. Falha numa parte não
// apaga as outras: o que deu certo fica, e o erro diz o que falhou.
func (m *Medidor) Medir(ctx context.Context, mbDown, mbUp int) Resultado {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	r := Resultado{Em: time.Now()}
	inicio := time.Now()
	var erros []string
	defer m.iniciarFase("", 0)
	m.iniciarFase("resposta", 0)

	if lat, srv, err := m.latencia(ctx); err != nil {
		erros = append(erros, "latência: "+err.Error())
	} else {
		r.LatenciaMS, r.Servidor = &lat, srv
	}
	m.iniciarFase("download", int64(mbDown)<<20)
	r.LatenciaDownloadMS = m.sobCarga(ctx, func() {
		if mbps, n, err := m.download(ctx, int64(mbDown)<<20); err != nil {
			erros = append(erros, "download: "+err.Error())
			r.BytesBaixados = n
		} else {
			r.DownloadMbps, r.BytesBaixados = &mbps, n
		}
	})
	m.iniciarFase("upload", int64(mbUp)<<20)
	r.LatenciaUploadMS = m.sobCarga(ctx, func() {
		if mbps, n, err := m.upload(ctx, int64(mbUp)<<20); err != nil {
			erros = append(erros, "upload: "+err.Error())
			r.BytesEnviados = n
		} else {
			r.UploadMbps, r.BytesEnviados = &mbps, n
		}
	})
	r.DuracaoS = time.Since(inicio).Seconds()
	r.Erro = strings.Join(erros, "; ")
	return r
}

// sobCarga roda a fase (download ou upload) e, enquanto ela dura, mede a
// resposta da internet com pedidos vazios numa conexão à parte. Devolve a
// mediana; nil se a fase foi rápida demais para medir.
func (m *Medidor) sobCarga(ctx context.Context, fase func()) *float64 {
	sctx, parar := context.WithCancel(ctx)
	var (
		mu     sync.Mutex
		tempos []float64
		wg     sync.WaitGroup
	)
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; sctx.Err() == nil; i++ {
			req, err := m.pedido(sctx, http.MethodGet, m.Base+"/__down?bytes=0", nil)
			if err != nil {
				return
			}
			t0 := time.Now()
			resp, err := m.sonda.Do(req)
			if err == nil {
				io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
				// o primeiro só abre a conexão; e não conta o que foi cortado no fim
				if i > 0 && sctx.Err() == nil && resp.StatusCode == http.StatusOK {
					mu.Lock()
					tempos = append(tempos, float64(time.Since(t0).Microseconds())/1000)
					mu.Unlock()
				}
			}
			select {
			case <-sctx.Done():
			case <-time.After(200 * time.Millisecond):
			}
		}
	}()
	fase()
	parar()
	wg.Wait()
	if len(tempos) == 0 {
		return nil
	}
	slices.Sort(tempos)
	md := float64(int64(tempos[len(tempos)/2]*10+0.5)) / 10
	return &md
}

// latencia: mediana de pedidos vazios na mesma conexão (o primeiro, que abre a conexão, não conta).
func (m *Medidor) latencia(ctx context.Context) (float64, string, error) {
	var tempos []float64
	servidor := ""
	for i := 0; i < 6; i++ {
		req, err := m.pedido(ctx, http.MethodGet, m.Base+"/__down?bytes=0", nil)
		if err != nil {
			return 0, "", err
		}
		t0 := time.Now()
		resp, err := m.cliente.Do(req)
		if err != nil {
			return 0, "", simplificar(err)
		}
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return 0, "", fmt.Errorf("servidor respondeu %d", resp.StatusCode)
		}
		if ray := resp.Header.Get("Cf-Ray"); ray != "" {
			if _, colo, ok := strings.Cut(ray, "-"); ok {
				servidor = colo
			}
		}
		if i > 0 {
			tempos = append(tempos, float64(time.Since(t0).Microseconds())/1000)
		}
	}
	slices.Sort(tempos)
	return tempos[len(tempos)/2], servidor, nil
}

// download: vários fluxos ao mesmo tempo; a taxa conta do primeiro byte ao último.
func (m *Medidor) download(ctx context.Context, total int64) (float64, int64, error) {
	por := total / int64(m.Fluxos)
	var (
		mu            sync.Mutex
		bytes         int64
		primeiro, fim time.Time
		erros         []error
		wg            sync.WaitGroup
	)
	for range m.Fluxos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req, err := m.pedido(ctx, http.MethodGet, fmt.Sprintf("%s/__down?bytes=%d", m.Base, por), nil)
			if err != nil {
				mu.Lock()
				erros = append(erros, err)
				mu.Unlock()
				return
			}
			resp, err := m.cliente.Do(req)
			if err != nil {
				mu.Lock()
				erros = append(erros, simplificar(err))
				mu.Unlock()
				return
			}
			defer resp.Body.Close()
			chegou := time.Now()
			n, err := io.Copy(io.Discard, &contador{r: resp.Body, total: &m.bytes})
			terminou := time.Now()
			mu.Lock()
			defer mu.Unlock()
			bytes += n
			if primeiro.IsZero() || chegou.Before(primeiro) {
				primeiro = chegou
			}
			if terminou.After(fim) {
				fim = terminou
			}
			if err != nil {
				erros = append(erros, simplificar(err))
			} else if resp.StatusCode != http.StatusOK {
				erros = append(erros, fmt.Errorf("servidor respondeu %d", resp.StatusCode))
			}
		}()
	}
	wg.Wait()
	if len(erros) > 0 {
		return 0, bytes, erros[0]
	}
	return mbps(bytes, fim.Sub(primeiro)), bytes, nil
}

func (m *Medidor) upload(ctx context.Context, total int64) (float64, int64, error) {
	fluxos := max(1, m.Fluxos/2)
	por := total / int64(fluxos)
	var (
		mu    sync.Mutex
		bytes int64
		erros []error
		wg    sync.WaitGroup
	)
	inicio := time.Now()
	for range fluxos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			corpo := &contador{r: io.LimitReader(lixo{}, por), total: &m.bytes}
			req, err := m.pedido(ctx, http.MethodPost, m.Base+"/__up", corpo)
			if err == nil {
				req.ContentLength = por
				req.Header.Set("Content-Type", "application/octet-stream")
				var resp *http.Response
				if resp, err = m.cliente.Do(req); err == nil {
					io.Copy(io.Discard, resp.Body)
					resp.Body.Close()
					if resp.StatusCode != http.StatusOK {
						err = fmt.Errorf("servidor respondeu %d", resp.StatusCode)
					}
				}
			}
			mu.Lock()
			defer mu.Unlock()
			bytes += corpo.n
			if err != nil {
				erros = append(erros, simplificar(err))
			}
		}()
	}
	wg.Wait()
	if len(erros) > 0 {
		return 0, bytes, erros[0]
	}
	return mbps(bytes, time.Since(inicio)), bytes, nil
}

func (m *Medidor) pedido(ctx context.Context, metodo, url string, corpo io.Reader) (*http.Request, error) {
	req, err := http.NewRequestWithContext(ctx, metodo, url, corpo)
	if err == nil {
		req.Header.Set("User-Agent", m.Agente)
	}
	return req, err
}

func mbps(bytes int64, d time.Duration) float64 {
	if d <= 0 {
		return 0
	}
	v := float64(bytes) * 8 / d.Seconds() / 1e6
	return float64(int64(v*10+0.5)) / 10
}

func simplificar(err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return errors.New("tempo esgotado")
	}
	if errors.Is(err, context.Canceled) {
		return errors.New("cancelado")
	}
	return err
}

// lixo gera bytes que não comprimem (padrão que se repete a cada 251 bytes).
type lixo struct{}

func (lixo) Read(p []byte) (int, error) {
	for i := range p {
		p[i] = byte(i*131 + i/251)
	}
	return len(p), nil
}

type contador struct {
	r     io.Reader
	n     int64
	total *atomic.Int64 // soma de todos os fluxos (progresso na tela)
}

func (c *contador) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n += int64(n)
	if c.total != nil {
		c.total.Add(int64(n))
	}
	return n, err
}
