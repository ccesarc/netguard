// Package dispositivos descobre os aparelhos da rede de casa.
//
// Duas formas: passiva (ler a tabela de vizinhos do kernel, sem enviar nada) e
// ativa (um ping em cada endereço da rede para os aparelhos se apresentarem).
// A ativa só roda com o dono autorizando nas Configurações. Nada de captura de
// tráfego: só IP, MAC e o nome que o roteador conhece.
package dispositivos

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/netip"
	"sync"
	"time"
)

// Observacao é um aparelho visto numa coleta.
type Observacao struct {
	MAC          net.HardwareAddr
	IP           netip.Addr
	Nome         string
	Fabricante   string
	Aleatorio    bool
	EsteAparelho bool
}

// Rede descreve a rede local no momento.
type Rede struct {
	Iface    string
	Endereco netip.Prefix // IP deste aparelho com o tamanho da rede (ex.: 10.0.0.209/24)
	Gateway  netip.Addr
	MAC      net.HardwareAddr // deste aparelho
}

type Armazem interface {
	// retrato = não avisar ninguém como "novo" (primeira procura de todas).
	RegistrarObservacoes(ctx context.Context, em time.Time, obs []Observacao, janela time.Duration, retrato bool) (novos int, err error)
	PrimeiraVarreduraFeita(ctx context.Context) (bool, error)
	MarcarPrimeiraVarredura(ctx context.Context) error
}

type Opcoes struct {
	Rede        func() (Rede, bool)
	LerVizinhos func(ctx context.Context, iface string) ([]Vizinho, error)
	Cutucar     func(ctx context.Context, ip netip.Addr) // um ping, sem esperar resposta útil
	NomeReverso func(ctx context.Context, servidor netip.AddrPort, ip netip.Addr) string
	Fabricantes interface {
		Buscar([]net.HardwareAddr) map[string]string
	}
	VarreduraAtiva func() bool
	Armazem        Armazem
	Log            *slog.Logger
}

const (
	// Janela de presença: visto há menos que isso = "na rede"; voltar depois disso abre outra aparição.
	Janela             = 10 * time.Minute
	intervaloVarredura = 5 * time.Minute
	maxHosts           = 1024
	paralelo           = 16
	blocoVarredura     = 64
)

// Progresso da procura em andamento (para a tela mostrar a barra).
type Progresso struct {
	Feitos  int       `json:"feitos"`
	Total   int       `json:"total"`
	Achados int       `json:"achados"` // aparelhos respondendo até agora
	Inicio  time.Time `json:"inicio"`
}

var ErrVarrendo = errors.New("já tem uma varredura rodando")
var ErrVarreduraCedo = errors.New("espere 1 minuto entre uma varredura e outra")

type Estado struct {
	UltimaColeta    *time.Time `json:"ultima_coleta"`
	UltimaVarredura *time.Time `json:"ultima_varredura"`
	// Início da última procura: quem foi visto antes disso não respondeu a ela.
	UltimaVarreduraInicio *time.Time `json:"ultima_varredura_inicio"`
	Varrendo              bool       `json:"varrendo"`
	VarreduraAtiva        bool       `json:"varredura_ativa"`
	Progresso             *Progresso `json:"progresso"`
}

type nomeGuardado struct {
	nome string
	em   time.Time
}

type Coletor struct {
	op    Opcoes
	agora func() time.Time

	mu                    sync.Mutex
	varrendo              bool
	progresso             *Progresso
	ultimaColeta          time.Time
	ultimaVarredura       time.Time
	ultimaVarreduraInicio time.Time
	nomes                 map[netip.Addr]nomeGuardado
}

func NovoColetor(op Opcoes) *Coletor {
	return &Coletor{op: op, agora: time.Now, nomes: map[netip.Addr]nomeGuardado{}}
}

func (c *Coletor) Estado() Estado {
	c.mu.Lock()
	defer c.mu.Unlock()
	e := Estado{Varrendo: c.varrendo, VarreduraAtiva: c.op.VarreduraAtiva()}
	if c.progresso != nil {
		p := *c.progresso
		e.Progresso = &p
	}
	if !c.ultimaColeta.IsZero() {
		t := c.ultimaColeta
		e.UltimaColeta = &t
	}
	if !c.ultimaVarredura.IsZero() {
		t := c.ultimaVarredura
		e.UltimaVarredura = &t
	}
	if !c.ultimaVarreduraInicio.IsZero() {
		t := c.ultimaVarreduraInicio
		e.UltimaVarreduraInicio = &t
	}
	return e
}

// Rodar coleta a cada minuto; com a varredura ativa ligada, varre a cada 5 min.
func (c *Coletor) Rodar(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	time.Sleep(3 * time.Second) // deixa o monitor medir a interface primeiro
	for {
		c.mu.Lock()
		varrer := c.op.VarreduraAtiva() && !c.varrendo && c.agora().Sub(c.ultimaVarredura) >= intervaloVarredura
		if varrer {
			c.varrendo = true
		}
		c.mu.Unlock()
		c.coletar(ctx, varrer)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// VarrerAgora roda uma varredura ativa em segundo plano (pedido pela tela).
func (c *Coletor) VarrerAgora(ctx context.Context) error {
	c.mu.Lock()
	if c.varrendo {
		c.mu.Unlock()
		return ErrVarrendo
	}
	if c.agora().Sub(c.ultimaVarredura) < time.Minute {
		c.mu.Unlock()
		return ErrVarreduraCedo
	}
	c.varrendo = true
	c.mu.Unlock()
	go c.coletar(context.WithoutCancel(ctx), true)
	return nil
}

func (c *Coletor) coletar(ctx context.Context, varrer bool) {
	if varrer {
		defer func() {
			c.mu.Lock()
			c.varrendo, c.progresso = false, nil
			c.mu.Unlock()
		}()
	}
	rede, ok := c.op.Rede()
	if !ok {
		return
	}
	// Na primeira procura de todas, quem aparecer é o retrato da casa: sem aviso de "novo",
	// mesmo nos registros parciais feitos durante a procura.
	retrato := false
	if varrer {
		if feita, err := c.op.Armazem.PrimeiraVarreduraFeita(ctx); err == nil && !feita {
			retrato = true
		}
		inicio := c.agora()
		c.varrer(ctx, rede, retrato)
		c.mu.Lock()
		c.ultimaVarredura, c.ultimaVarreduraInicio = c.agora(), inicio
		c.mu.Unlock()
		time.Sleep(time.Second) // respostas ARP atrasadas
	}
	c.registrar(ctx, rede, retrato)
	if retrato {
		if err := c.op.Armazem.MarcarPrimeiraVarredura(ctx); err != nil {
			c.op.Log.Warn("não marcou a primeira procura", "erro", err)
		}
	}
}

// registrar lê a tabela de vizinhos e grava quem está presente. Devolve quantos.
func (c *Coletor) registrar(ctx context.Context, rede Rede, retrato bool) int {
	vizinhos, err := c.op.LerVizinhos(ctx, rede.Iface)
	if err != nil {
		c.op.Log.Warn("não leu os vizinhos", "erro", err)
		return 0
	}
	obs := c.observacoes(ctx, rede, vizinhos)
	agora := c.agora()
	novos, err := c.op.Armazem.RegistrarObservacoes(ctx, agora, obs, Janela, retrato)
	if err != nil {
		c.op.Log.Warn("não gravou os aparelhos", "erro", err)
		return len(obs)
	}
	c.mu.Lock()
	c.ultimaColeta = agora
	c.mu.Unlock()
	if novos > 0 {
		c.op.Log.Warn("aparelho novo na rede", "quantos", novos)
	}
	return len(obs)
}

// observacoes junta os vizinhos confirmados, este aparelho, nome e fabricante.
func (c *Coletor) observacoes(ctx context.Context, rede Rede, vizinhos []Vizinho) []Observacao {
	var obs []Observacao
	vistos := map[string]bool{}
	for _, v := range vizinhos {
		if !v.Confirmado() || !rede.Endereco.Contains(v.IP) || vistos[v.MAC.String()] {
			continue
		}
		vistos[v.MAC.String()] = true
		obs = append(obs, Observacao{MAC: v.MAC, IP: v.IP, Aleatorio: MACAleatorio(v.MAC)})
	}
	if len(rede.MAC) > 0 && !vistos[rede.MAC.String()] {
		obs = append(obs, Observacao{MAC: rede.MAC, IP: rede.Endereco.Addr(), Aleatorio: MACAleatorio(rede.MAC), EsteAparelho: true})
	}

	macs := make([]net.HardwareAddr, len(obs))
	for i, o := range obs {
		macs[i] = o.MAC
	}
	fab := c.op.Fabricantes.Buscar(macs)
	for i := range obs {
		obs[i].Fabricante = fab[obs[i].MAC.String()]
	}
	c.preencherNomes(ctx, rede, obs)
	return obs
}

// preencherNomes pergunta ao roteador o nome de cada IP; guarda por 1 h.
func (c *Coletor) preencherNomes(ctx context.Context, rede Rede, obs []Observacao) {
	if !rede.Gateway.IsValid() {
		return
	}
	srv := netip.AddrPortFrom(rede.Gateway, 53)
	var wg sync.WaitGroup
	limite := make(chan struct{}, 8)
	for i := range obs {
		c.mu.Lock()
		g, ok := c.nomes[obs[i].IP]
		c.mu.Unlock()
		if ok && c.agora().Sub(g.em) < time.Hour {
			obs[i].Nome = g.nome
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			limite <- struct{}{}
			defer func() { <-limite }()
			nome := c.op.NomeReverso(ctx, srv, obs[i].IP)
			obs[i].Nome = nome
			c.mu.Lock()
			c.nomes[obs[i].IP] = nomeGuardado{nome, c.agora()}
			c.mu.Unlock()
		}()
	}
	wg.Wait()
}

// varrer cutuca cada endereço da rede (no máximo 1024), 16 de cada vez, em
// blocos de 64: depois de cada bloco grava quem já respondeu, para a tela ir
// mostrando os aparelhos enquanto a procura anda.
func (c *Coletor) varrer(ctx context.Context, rede Rede, retrato bool) {
	p := rede.Endereco.Masked()
	if p.Bits() < 22 {
		c.op.Log.Warn("rede grande demais para varrer inteira; só os primeiros 1024 endereços", "rede", p.String())
	}
	var alvos []netip.Addr
	for ip := p.Addr().Next(); p.Contains(ip) && len(alvos) < maxHosts; ip = ip.Next() {
		if ip != rede.Endereco.Addr() && p.Contains(ip.Next()) { // pula este aparelho e o broadcast
			alvos = append(alvos, ip)
		}
	}
	c.mu.Lock()
	c.progresso = &Progresso{Total: len(alvos), Inicio: c.agora()}
	c.mu.Unlock()

	limite := make(chan struct{}, paralelo)
	for ini := 0; ini < len(alvos) && ctx.Err() == nil; ini += blocoVarredura {
		bloco := alvos[ini:min(ini+blocoVarredura, len(alvos))]
		var wg sync.WaitGroup
		for _, ip := range bloco {
			wg.Add(1)
			limite <- struct{}{}
			go func(ip netip.Addr) {
				defer wg.Done()
				defer func() { <-limite }()
				c.op.Cutucar(ctx, ip)
			}(ip)
		}
		wg.Wait()
		achados := c.registrar(ctx, rede, retrato)
		c.mu.Lock()
		if c.progresso != nil {
			c.progresso.Feitos = ini + len(bloco)
			c.progresso.Achados = achados
		}
		c.mu.Unlock()
	}
	c.op.Log.Info("varredura feita", "enderecos", len(alvos))
}
