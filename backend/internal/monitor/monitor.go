// Package monitor roda as verificações da rede em segundo plano, sem depender
// de página aberta: interface deste aparelho, roteador, IPs de fora, DNS e
// sites. Cada ciclo vira um diagnóstico e as quedas são registradas.
package monitor

import (
	"context"
	"log/slog"
	"net/netip"
	"sync"
	"time"

	"netguard/internal/ajustes"
	"netguard/internal/armazenamento"
	"netguard/internal/diagnostico"
	"netguard/internal/incidentes"
)

type MedidaPing struct {
	Alvo      string    `json:"alvo"`
	OK        bool      `json:"ok"`
	Enviados  int       `json:"enviados"`
	Recebidos int       `json:"recebidos"`
	Perda     float64   `json:"perda"`
	MinMS     float64   `json:"latencia_min_ms"`
	MediaMS   float64   `json:"latencia_media_ms"`
	MaxMS     float64   `json:"latencia_max_ms"`
	ViaTCP    bool      `json:"via_tcp,omitempty"` // ping bloqueado, mas conectou na 443
	Erro      string    `json:"erro,omitempty"`
	MedidoEm  time.Time `json:"medido_em"`
}

// Medida serve para DNS e HTTP: deu certo ou não, em quanto tempo.
type Medida struct {
	Alvo   string  `json:"alvo"`
	OK     bool    `json:"ok"`
	MS     float64 `json:"ms"`
	Status int     `json:"status,omitempty"`
	Erro   string  `json:"erro,omitempty"`
}

type Estado struct {
	MedidoEm      time.Time             `json:"medido_em"`
	Interface     EstadoInterface       `json:"interface"`
	ErroInterface string                `json:"erro_interface,omitempty"`
	Gateway       *MedidaPing           `json:"gateway,omitempty"`
	Externos      []MedidaPing          `json:"externos,omitempty"`
	DNSSistema    []Medida              `json:"dns_sistema,omitempty"`
	DNSExterno    []Medida              `json:"dns_externo,omitempty"`
	HTTP          []Medida              `json:"http,omitempty"`
	Diagnostico   diagnostico.Resultado `json:"diagnostico"`
	Queda         *incidentes.Incidente `json:"queda,omitempty"`
}

type Armazem interface {
	SalvarCiclo(ctx context.Context, c armazenamento.Ciclo) error
	IniciarExecucao(ctx context.Context, inicio time.Time, versao string) (int64, error)
	BaterExecucao(ctx context.Context, id int64, em time.Time) error
	UltimaExecucaoFim(ctx context.Context) (time.Time, bool, error)
	IncidenteAberto(ctx context.Context) (*incidentes.Incidente, error)
	SalvarIncidente(ctx context.Context, inc incidentes.Incidente) (int64, error)
}

type Opcoes struct {
	Interface     string
	Intervalo     time.Duration
	LerInterface  func(nome string) (EstadoInterface, error)
	Pinger        Pinger
	Conector      Conector
	Resolvedor    Resolvedor
	HTTP          TestadorHTTP
	ServidoresDNS func() []netip.AddrPort
	Armazem       Armazem
	Log           *slog.Logger
	Versao        string
	Ocupado       func() bool // teste de velocidade rodando: latência alta é dele, não da internet
	// Pausado: busca de redes Wi-Fi em andamento. A placa sai do canal por 1–2 s
	// e tudo falha junto; o ciclo que pega isso é descartado (não é a internet).
	Pausado func() bool

	Externos   []netip.Addr     // IPs públicos para ping
	DNSExterno []netip.AddrPort // DNS independente, para comparar
	NomesDNS   []string         // nomes consultados (um por ciclo, em rodízio)
	URLs       []string         // sites testados por HTTPS
	Limites    diagnostico.Limites
}

var (
	ExternosPadrao   = []netip.Addr{netip.MustParseAddr("1.1.1.1"), netip.MustParseAddr("8.8.8.8"), netip.MustParseAddr("9.9.9.9")}
	DNSExternoPadrao = []netip.AddrPort{netip.MustParseAddrPort("1.1.1.1:53")}
	NomesDNSPadrao   = []string{"google.com", "cloudflare.com", "wikipedia.org"}
	URLsPadrao       = []string{"https://www.google.com/generate_204", "https://www.cloudflare.com/cdn-cgi/trace"}
)

const (
	pacotesGateway   = 5
	pacotesExterno   = 3
	ciclosParaAbrir  = 3
	ciclosParaFechar = 2
)

type Monitor struct {
	op         Opcoes
	cfgMu      sync.RWMutex // protege os campos de op que a tela de Configurações muda
	agora      func() time.Time
	rastreador *incidentes.Rastreador
	execucao   int64
	rodizio    int

	mu        sync.RWMutex
	estado    Estado
	temEstado bool
}

func Novo(op Opcoes) *Monitor {
	if op.Externos == nil {
		op.Externos = ExternosPadrao
	}
	if op.DNSExterno == nil {
		op.DNSExterno = DNSExternoPadrao
	}
	if op.NomesDNS == nil {
		op.NomesDNS = NomesDNSPadrao
	}
	if op.URLs == nil {
		op.URLs = URLsPadrao
	}
	if op.Limites == (diagnostico.Limites{}) {
		op.Limites = diagnostico.LimitesPadrao
	}
	if op.ServidoresDNS == nil {
		op.ServidoresDNS = ServidoresDNS
	}
	if op.Ocupado == nil {
		op.Ocupado = func() bool { return false }
	}
	if op.Pausado == nil {
		op.Pausado = func() bool { return false }
	}
	return &Monitor{op: op, agora: time.Now, rastreador: incidentes.NovoRastreador(ciclosParaAbrir, ciclosParaFechar)}
}

// Configurar aplica o que foi salvo na tela; vale a partir do próximo ciclo.
// a já vem validado (ajustes.Validar).
func (m *Monitor) Configurar(a ajustes.Ajustes) {
	var externos []netip.Addr
	for _, e := range a.Externos {
		if ip, err := netip.ParseAddr(e); err == nil {
			externos = append(externos, ip)
		}
	}
	dns := DNSExternoPadrao
	if ip, err := netip.ParseAddr(a.DNSComparacao); err == nil {
		dns = []netip.AddrPort{netip.AddrPortFrom(ip, 53)}
	}
	m.cfgMu.Lock()
	defer m.cfgMu.Unlock()
	m.op.Intervalo = time.Duration(a.IntervaloS) * time.Second
	m.op.Externos = externos
	m.op.DNSExterno = dns
	m.op.URLs = append([]string{}, a.URLs...)
	m.op.Limites = diagnostico.Limites{LatenciaMS: a.LimiteLatenciaMS, Perda: a.LimitePerda}
}

// alvos é a cópia da configuração usada num ciclo.
type alvos struct {
	intervalo  time.Duration
	externos   []netip.Addr
	dnsExterno []netip.AddrPort
	urls       []string
	limites    diagnostico.Limites
}

func (m *Monitor) alvos() alvos {
	m.cfgMu.RLock()
	defer m.cfgMu.RUnlock()
	return alvos{m.op.Intervalo, m.op.Externos, m.op.DNSExterno, m.op.URLs, m.op.Limites}
}

// Rodar mede já e depois a cada intervalo, até o ctx acabar.
func (m *Monitor) Rodar(ctx context.Context) {
	m.op.Log.Info("monitor iniciado", "intervalo", m.op.Intervalo.String(), "interface", m.op.Interface)
	m.retomar(ctx)
	for {
		inicio := time.Now()
		intervalo := m.alvos().intervalo
		cctx, cancel := context.WithTimeout(ctx, intervalo)
		m.ciclo(cctx)
		cancel()
		select {
		case <-ctx.Done():
			m.op.Log.Info("monitor parado")
			return
		case <-time.After(intervalo - time.Since(inicio)):
		}
	}
}

// retomar fecha uma queda que ficou aberta quando o serviço parou. Não dá para
// saber o que houve com o monitor desligado: ela termina na última vez em que ele
// estava rodando. Se a rede ainda estiver fora, uma queda nova abre.
func (m *Monitor) retomar(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	inc, err := m.op.Armazem.IncidenteAberto(ctx)
	if err != nil {
		m.op.Log.Warn("não deu para conferir queda aberta", "erro", err)
	} else if inc != nil {
		fim, ok, _ := m.op.Armazem.UltimaExecucaoFim(ctx)
		if !ok || fim.Before(inc.Inicio) {
			fim = inc.Inicio
		}
		inc.Fim = &fim
		inc.Detalhe = juntarTexto(inc.Detalhe, "encerrada quando o monitor parou")
		if _, err := m.op.Armazem.SalvarIncidente(ctx, *inc); err != nil {
			m.op.Log.Warn("não fechou a queda antiga", "erro", err)
		} else {
			m.op.Log.Info("queda antiga encerrada", "id", inc.ID, "fim", fim)
		}
	}
	m.iniciarExecucao(ctx)
}

func (m *Monitor) iniciarExecucao(ctx context.Context) {
	id, err := m.op.Armazem.IniciarExecucao(ctx, m.agora(), m.op.Versao)
	if err != nil {
		m.op.Log.Warn("não registrou o início do monitor", "erro", err)
		return
	}
	m.execucao = id
}

// Estado devolve a última medição; ok=false se ainda não mediu nada.
func (m *Monitor) Estado() (Estado, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.estado, m.temEstado
}

func (m *Monitor) ciclo(ctx context.Context) Estado {
	pausadoNoInicio := m.op.Pausado()
	agora := m.agora()
	est := Estado{MedidoEm: agora}
	ent := diagnostico.Entrada{}
	var vs []armazenamento.Verificacao

	iface, err := m.op.LerInterface(m.op.Interface)
	est.Interface = iface
	switch {
	case err != nil:
		est.ErroInterface = err.Error()
	case !iface.Ativa:
		est.ErroInterface = "interface desligada ou sem sinal"
	case iface.IPv4 == "":
		est.ErroInterface = "interface sem endereço IPv4"
	case !iface.TemRota():
		est.ErroInterface = "sem rota padrão por esta interface"
	}
	ent.InterfaceOK, ent.InterfaceProblema = est.ErroInterface == "", est.ErroInterface
	vi := armazenamento.Verificacao{Em: agora, Tipo: "interface", Alvo: iface.Nome, OK: ent.InterfaceOK, Erro: est.ErroInterface,
		Dados: map[string]any{"tipo": iface.Tipo, "ipv4": iface.IPv4}}
	if vi.Alvo == "" {
		vi.Alvo = "(rota padrão)"
	}
	if iface.TemRota() {
		vi.Dados["gateway"] = iface.Gateway.String()
	}
	vs = append(vs, vi)

	cfg := m.alvos()
	// Com a placa fora, o resto só falharia junto: não mede, a causa já é conhecida.
	if ent.InterfaceOK {
		m.medirRede(ctx, cfg, iface, &est, &ent)
		vs = append(vs, verificacoes(agora, &est)...)
	}

	limites := cfg.limites
	ocupado := m.op.Ocupado()
	if ocupado {
		limites = diagnostico.Limites{LatenciaMS: 1e9, Perda: 1}
	}
	est.Diagnostico = diagnostico.Diagnosticar(ent, limites)
	if ocupado {
		est.Diagnostico.Detalhe = juntarTexto(est.Diagnostico.Detalhe, "durante teste de velocidade")
	}
	if pausadoNoInicio || m.op.Pausado() {
		// Não grava nem conta para quedas; o estado da tela fica o do ciclo anterior.
		m.op.Log.Debug("ciclo descartado: busca de redes Wi-Fi em andamento")
		if e, ok := m.Estado(); ok {
			return e
		}
		return est
	}
	m.registrarQuedas(ctx, agora, est.Diagnostico)
	est.Queda = m.rastreador.Aberto()

	m.mu.Lock()
	m.estado, m.temEstado = est, true
	m.mu.Unlock()

	// Gravar não pode travar a medição seguinte nem derrubar o monitor.
	sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if err := m.op.Armazem.SalvarCiclo(sctx, armazenamento.Ciclo{
		Em: agora, Estado: string(est.Diagnostico.Estado), Causa: string(est.Diagnostico.Causa),
		Detalhe: est.Diagnostico.Detalhe, Verificacoes: vs,
	}); err != nil {
		m.op.Log.Warn("não gravou o ciclo", "erro", err)
	}
	if m.execucao == 0 {
		m.iniciarExecucao(sctx)
	} else if err := m.op.Armazem.BaterExecucao(sctx, m.execucao, agora); err != nil {
		m.op.Log.Warn("não registrou o batimento", "erro", err)
	}
	return est
}

// medirRede roda todas as sondas ao mesmo tempo.
func (m *Monitor) medirRede(ctx context.Context, cfg alvos, iface EstadoInterface, est *Estado, ent *diagnostico.Entrada) {
	var wg sync.WaitGroup
	servidores := m.op.ServidoresDNS()
	nome := m.op.NomesDNS[m.rodizio%len(m.op.NomesDNS)]
	m.rodizio++

	est.Externos = make([]MedidaPing, len(cfg.externos))
	est.DNSSistema = make([]Medida, len(servidores))
	est.DNSExterno = make([]Medida, len(cfg.dnsExterno))
	est.HTTP = make([]Medida, len(cfg.urls))

	if iface.TemRota() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mp := medida(iface.Gateway.String(), m.op.Pinger.Ping(ctx, iface.Gateway, pacotesGateway), m.agora())
			est.Gateway = &mp
		}()
	}
	for i, ip := range cfg.externos {
		wg.Add(1)
		go func() {
			defer wg.Done()
			mp := medida(ip.String(), m.op.Pinger.Ping(ctx, ip, pacotesExterno), m.agora())
			if !mp.OK && ctx.Err() == nil { // ping pode estar bloqueado no caminho: tenta TCP 443
				tctx, cancel := context.WithTimeout(ctx, 2*time.Second)
				d, err := m.op.Conector.Conectar(tctx, netip.AddrPortFrom(ip, 443))
				cancel()
				if err == nil {
					mp = MedidaPing{Alvo: mp.Alvo, OK: true, ViaTCP: true, MediaMS: ms(d), MinMS: ms(d), MaxMS: ms(d), MedidoEm: m.agora()}
				}
			}
			est.Externos[i] = mp
		}()
	}
	consultar := func(destino *Medida, srv netip.AddrPort) {
		defer wg.Done()
		dctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		defer cancel()
		d, err := m.op.Resolvedor.Resolver(dctx, srv, nome)
		*destino = Medida{Alvo: srv.Addr().String(), OK: err == nil, MS: ms(d)}
		if err != nil {
			destino.Erro = err.Error()
		}
	}
	for i, srv := range servidores {
		wg.Add(1)
		go consultar(&est.DNSSistema[i], srv)
	}
	for i, srv := range cfg.dnsExterno {
		wg.Add(1)
		go consultar(&est.DNSExterno[i], srv)
	}
	for i, url := range cfg.urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			hctx, cancel := context.WithTimeout(ctx, 6*time.Second)
			defer cancel()
			d, status, err := m.op.HTTP.Testar(hctx, url)
			est.HTTP[i] = Medida{Alvo: url, OK: err == nil, MS: ms(d), Status: status}
			if err != nil {
				est.HTTP[i].Erro = err.Error()
			}
		}()
	}
	wg.Wait()

	ent.GatewayMedido = est.Gateway != nil
	if est.Gateway != nil {
		ent.Gateway = sondaPing(*est.Gateway)
	}
	for _, e := range est.Externos {
		ent.Externos = append(ent.Externos, sondaPing(e))
	}
	ent.DNSSistema = sondas(est.DNSSistema)
	ent.DNSExterno = sondas(est.DNSExterno)
	ent.HTTP = sondas(est.HTTP)
}

func (m *Monitor) registrarQuedas(ctx context.Context, em time.Time, res diagnostico.Resultado) {
	for _, mud := range m.rastreador.Observar(em, res) {
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		id, err := m.op.Armazem.SalvarIncidente(sctx, mud.Incidente)
		cancel()
		if err != nil {
			m.op.Log.Warn("não gravou a queda", "erro", err)
			continue
		}
		if mud.Tipo != incidentes.Fechou {
			m.rastreador.DefinirID(id)
		}
		switch mud.Tipo {
		case incidentes.Abriu:
			m.op.Log.Warn("queda", "causa", mud.Incidente.Causa, "detalhe", mud.Incidente.Detalhe, "inicio", mud.Incidente.Inicio)
		case incidentes.MudouCausa:
			m.op.Log.Warn("queda mudou de causa", "causa", mud.Incidente.Causa)
		case incidentes.Fechou:
			m.op.Log.Info("recuperou", "duracao", mud.Incidente.Fim.Sub(mud.Incidente.Inicio).String())
		}
	}
}

func verificacoes(em time.Time, est *Estado) []armazenamento.Verificacao {
	var vs []armazenamento.Verificacao
	ping := func(tipo string, mp MedidaPing) {
		v := armazenamento.Verificacao{Em: em, Tipo: tipo, Alvo: mp.Alvo, OK: mp.OK, Erro: mp.Erro}
		perda := mp.Perda
		v.Perda = &perda
		if mp.OK {
			lat := mp.MediaMS
			v.LatenciaMS = &lat
		}
		if mp.ViaTCP {
			v.Dados = map[string]any{"via": "tcp443"}
		}
		vs = append(vs, v)
	}
	simples := func(tipo string, md Medida) {
		v := armazenamento.Verificacao{Em: em, Tipo: tipo, Alvo: md.Alvo, OK: md.OK, Erro: md.Erro}
		if md.OK {
			lat := md.MS
			v.LatenciaMS = &lat
		}
		if md.Status != 0 {
			v.Dados = map[string]any{"status": md.Status}
		}
		vs = append(vs, v)
	}
	if est.Gateway != nil {
		ping("gateway", *est.Gateway)
	}
	for _, e := range est.Externos {
		ping("externo", e)
	}
	for _, d := range est.DNSSistema {
		simples("dns_sistema", d)
	}
	for _, d := range est.DNSExterno {
		simples("dns_externo", d)
	}
	for _, h := range est.HTTP {
		simples("http", h)
	}
	return vs
}

func sondaPing(mp MedidaPing) diagnostico.Sonda {
	return diagnostico.Sonda{Alvo: mp.Alvo, OK: mp.OK, LatenciaMS: mp.MediaMS, Perda: mp.Perda}
}

func sondas(ms []Medida) []diagnostico.Sonda {
	r := make([]diagnostico.Sonda, len(ms))
	for i, x := range ms {
		r[i] = diagnostico.Sonda{Alvo: x.Alvo, OK: x.OK, LatenciaMS: x.MS}
		if !x.OK {
			r[i].Perda = 1
		}
	}
	return r
}

func medida(alvo string, r ResultadoPing, em time.Time) MedidaPing {
	mn, md, mx := r.Estatisticas()
	return MedidaPing{
		Alvo:      alvo,
		OK:        r.Recebidos > 0,
		Enviados:  r.Enviados,
		Recebidos: r.Recebidos,
		Perda:     r.Perda(),
		MinMS:     ms(mn),
		MediaMS:   ms(md),
		MaxMS:     ms(mx),
		Erro:      r.Erro,
		MedidoEm:  em,
	}
}

func ms(d time.Duration) float64 { return float64(d.Microseconds()) / 1000 }

func juntarTexto(a, b string) string {
	if a == "" {
		return b
	}
	return a + " · " + b
}
