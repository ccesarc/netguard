// Package wifi lê a qualidade do Wi-Fi pelo NetworkManager (nmcli).
//
// O driver deste PC (broadcom wl) não informa sinal pelo kernel (nem iw nem
// /proc/net/wireless), então a fonte é o nmcli. O comando é fixo; só o nome da
// interface entra, e ele já vem validado da configuração. Não força busca de
// redes (--rescan no): buscar derruba a velocidade do Wi-Fi por alguns segundos.
package wifi

import (
	"context"
	"errors"
	"log/slog"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Amostra struct {
	Em              time.Time `json:"em"`
	SSID            string    `json:"ssid"`
	Sinal           int       `json:"sinal"` // 0 a 100, como o NetworkManager mostra
	Canal           int       `json:"canal"`
	FreqMHz         int       `json:"freq_mhz"`
	TaxaMbps        int       `json:"taxa_mbps"` // velocidade da ligação com o roteador, não da internet
	Seguranca       string    `json:"seguranca,omitempty"`
	RedesMesmoCanal int       `json:"redes_mesmo_canal"`
	RedesVistas     int       `json:"redes_vistas"`
}

var ErrNaoConectado = errors.New("não está conectado a nenhuma rede Wi-Fi")

type Leitor interface {
	Ler(ctx context.Context, iface string) (Amostra, error)
}

type LeitorNmcli struct{}

func (LeitorNmcli) Ler(ctx context.Context, iface string) (Amostra, error) {
	args := []string{"-t", "-f", "IN-USE,SSID,CHAN,FREQ,RATE,SIGNAL,SECURITY", "dev", "wifi", "list", "--rescan", "no"}
	if iface != "" {
		args = append(args, "ifname", iface)
	}
	cmd := exec.CommandContext(ctx, "nmcli", args...)
	cmd.Env = []string{"LC_ALL=C", "PATH=/usr/bin:/bin"} // números e unidades sem tradução
	saida, err := cmd.Output()
	if err != nil {
		var ee *exec.Error
		if errors.As(err, &ee) {
			return Amostra{}, errors.New("nmcli não encontrado (NetworkManager não instalado?)")
		}
		return Amostra{}, errors.New("nmcli falhou: " + err.Error())
	}
	return analisar(string(saida))
}

// analisar lê a saída "-t" do nmcli: campos separados por ":", com ":" e "\"
// escapados por "\" dentro dos valores (o SSID pode ter dois-pontos).
func analisar(saida string) (Amostra, error) {
	var atual *Amostra
	var todas []Amostra
	for _, linha := range strings.Split(strings.TrimSpace(saida), "\n") {
		c := campos(linha)
		if len(c) < 7 {
			continue
		}
		a := Amostra{
			SSID:      c[1],
			Canal:     inteiro(c[2]),
			FreqMHz:   inteiro(c[3]),
			TaxaMbps:  inteiro(c[4]),
			Sinal:     min(100, max(0, inteiro(c[5]))),
			Seguranca: c[6],
		}
		todas = append(todas, a)
		if c[0] == "*" && atual == nil {
			x := a
			atual = &x
		}
	}
	if atual == nil {
		return Amostra{}, ErrNaoConectado
	}
	atual.RedesVistas = len(todas) - 1
	for _, a := range todas {
		if a.Canal == atual.Canal && a.SSID != atual.SSID {
			atual.RedesMesmoCanal++
		}
	}
	return *atual, nil
}

func campos(linha string) []string {
	var r []string
	var b strings.Builder
	escapado := false
	for _, ch := range linha {
		switch {
		case escapado:
			b.WriteRune(ch)
			escapado = false
		case ch == '\\':
			escapado = true
		case ch == ':':
			r = append(r, b.String())
			b.Reset()
		default:
			b.WriteRune(ch)
		}
	}
	return append(r, b.String())
}

// inteiro pega o número do começo: "5240 MHz" → 5240, "1170 Mbit/s" → 1170.
func inteiro(s string) int {
	s = strings.TrimSpace(s)
	fim := 0
	for fim < len(s) && s[fim] >= '0' && s[fim] <= '9' {
		fim++
	}
	n, _ := strconv.Atoi(s[:fim])
	return n
}

type Armazem interface {
	SalvarWifi(ctx context.Context, a Amostra) error
}

// Coletor lê o Wi-Fi a cada minuto e guarda a última leitura.
type Coletor struct {
	leitor  Leitor
	armazem Armazem
	iface   func() (nome, tipo string)
	log     *slog.Logger

	mu     sync.RWMutex
	ultima *Amostra
	motivo string
}

func NovoColetor(l Leitor, a Armazem, iface func() (string, string), log *slog.Logger) *Coletor {
	return &Coletor{leitor: l, armazem: a, iface: iface, log: log, motivo: "Aguardando a primeira leitura."}
}

// Ultima devolve a última leitura ou o motivo de não haver uma.
func (c *Coletor) Ultima() (*Amostra, string) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.ultima, c.motivo
}

func (c *Coletor) Rodar(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		c.coletar(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Coletor) coletar(ctx context.Context) {
	nome, tipo := c.iface()
	if tipo != "wifi" {
		c.definir(nil, "Este aparelho não está usando Wi-Fi (cabo ou sem rede).")
		return
	}
	lctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	a, err := c.leitor.Ler(lctx, nome)
	cancel()
	if err != nil {
		c.definir(nil, "Não deu para ler o Wi-Fi: "+err.Error())
		return
	}
	a.Em = time.Now()
	c.definir(&a, "")
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
	defer cancel()
	if err := c.armazem.SalvarWifi(sctx, a); err != nil {
		c.log.Warn("não gravou a leitura do Wi-Fi", "erro", err)
	}
}

func (c *Coletor) definir(a *Amostra, motivo string) {
	c.mu.Lock()
	c.ultima, c.motivo = a, motivo
	c.mu.Unlock()
}
