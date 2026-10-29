package velocidade

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"netguard/internal/ajustes"
)

var (
	ErrRodando = errors.New("já tem um teste rodando")
	ErrCedo    = errors.New("espere 2 minutos entre um teste e outro")
)

type Medicao interface {
	Medir(ctx context.Context, mbDown, mbUp int) Resultado
}

type Armazem interface {
	SalvarTesteVelocidade(ctx context.Context, r Resultado) error
}

type Estado struct {
	Rodando   bool            `json:"rodando"`
	Proximo   *time.Time      `json:"proximo"`
	Ultimo    *Resultado      `json:"ultimo"`
	Progresso *ProgressoTeste `json:"progresso"`
}

// comProgresso é o medidor que sabe dizer como o teste está indo.
type comProgresso interface{ Progresso() *ProgressoTeste }

type Agendador struct {
	medicao    Medicao
	armazem    Armazem
	ajustes    func() ajustes.Velocidade
	internetOK func() bool   // não testa com a internet fora
	iface      func() string // nome da interface no momento do teste
	log        *slog.Logger
	agora      func() time.Time

	mu           sync.Mutex
	rodando      bool
	ultimo       *Resultado
	ultimoInicio time.Time
	ctx          context.Context
}

func NovoAgendador(m Medicao, a Armazem, aj func() ajustes.Velocidade, internetOK func() bool, iface func() string, log *slog.Logger) *Agendador {
	return &Agendador{medicao: m, armazem: a, ajustes: aj, internetOK: internetOK, iface: iface, log: log, agora: time.Now, ctx: context.Background()}
}

// Ocupado diz se um teste está rodando: o monitor usa para não chamar de
// "internet lenta" a latência alta que o próprio teste provoca.
func (g *Agendador) Ocupado() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.rodando
}

func (g *Agendador) Estado() Estado {
	g.mu.Lock()
	defer g.mu.Unlock()
	e := Estado{Rodando: g.rodando, Ultimo: g.ultimo}
	if p, ok := g.medicao.(comProgresso); ok && g.rodando {
		e.Progresso = p.Progresso()
	}
	if v := g.ajustes(); v.Ativo {
		p := ProximoHorario(g.agora(), v.VezesPorDia)
		e.Proximo = &p
	}
	return e
}

// DefinirUltimo carrega o último teste do banco ao iniciar.
func (g *Agendador) DefinirUltimo(r *Resultado) {
	g.mu.Lock()
	g.ultimo = r
	g.mu.Unlock()
}

// Rodar confere a cada minuto se chegou a hora de um teste agendado.
func (g *Agendador) Rodar(ctx context.Context) {
	g.mu.Lock()
	g.ctx = ctx
	g.mu.Unlock()
	proximo := ProximoHorario(g.agora(), max(1, g.ajustes().VezesPorDia))
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		v := g.ajustes()
		agora := g.agora()
		if agora.Before(proximo) {
			// a quantidade por dia pode ter mudado nas Configurações
			if novo := ProximoHorario(agora, max(1, v.VezesPorDia)); novo.Before(proximo) {
				proximo = novo
			}
			continue
		}
		proximo = ProximoHorario(agora.Add(time.Minute), max(1, v.VezesPorDia))
		if !v.Ativo {
			continue
		}
		if !g.internetOK() {
			g.log.Info("teste de velocidade agendado pulado: internet fora")
			continue
		}
		if err := g.iniciar("agendado", false); err != nil {
			g.log.Info("teste de velocidade agendado pulado", "motivo", err)
		}
	}
}

// TestarAgora começa um teste em segundo plano (pedido pela tela).
func (g *Agendador) TestarAgora() error {
	return g.iniciar("manual", true)
}

func (g *Agendador) iniciar(origem string, conferirIntervalo bool) error {
	g.mu.Lock()
	if g.rodando {
		g.mu.Unlock()
		return ErrRodando
	}
	if conferirIntervalo && !g.ultimoInicio.IsZero() && g.agora().Sub(g.ultimoInicio) < 2*time.Minute {
		g.mu.Unlock()
		return ErrCedo
	}
	g.rodando = true
	g.ultimoInicio = g.agora()
	ctx := g.ctx
	g.mu.Unlock()

	go func() {
		v := g.ajustes()
		g.log.Info("teste de velocidade começou", "origem", origem)
		r := g.medicao.Medir(ctx, v.MBDownload, v.MBUpload)
		r.Origem, r.Interface = origem, g.iface()
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		if err := g.armazem.SalvarTesteVelocidade(sctx, r); err != nil {
			g.log.Warn("não gravou o teste de velocidade", "erro", err)
		}
		cancel()
		g.log.Info("teste de velocidade terminou", "download_mbps", r.DownloadMbps, "upload_mbps", r.UploadMbps, "erro", r.Erro)
		g.mu.Lock()
		g.rodando, g.ultimo = false, &r
		g.mu.Unlock()
	}()
	return nil
}

// ProximoHorario espalha os testes no dia a partir das 7h (hora local):
// 3 por dia = 7h, 15h e 23h. Devolve o primeiro depois de agora.
func ProximoHorario(agora time.Time, vezes int) time.Time {
	vezes = max(1, vezes)
	passo := 24 * time.Hour / time.Duration(vezes)
	ano, mes, dia := agora.Date()
	base := time.Date(ano, mes, dia, 7, 0, 0, 0, agora.Location())
	if base.After(agora) {
		base = base.Add(-24 * time.Hour)
	}
	for t := base; ; t = t.Add(passo) {
		if t.After(agora) {
			return t
		}
	}
}
