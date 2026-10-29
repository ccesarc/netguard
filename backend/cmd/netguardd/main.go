// netguardd: monitor da rede de casa (API, verificações em segundo plano e painel).
//
//	netguardd [servir]               roda o serviço
//	netguardd migrar                 só aplica as migrações
//	netguardd usuario criar <nome>   cria um usuário (pede a senha)
//	netguardd usuario senha <nome>   troca a senha e derruba as sessões
//	netguardd versao
package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"golang.org/x/term"

	"netguard/internal/ajustes"
	"netguard/internal/api"
	"netguard/internal/armazenamento"
	"netguard/internal/auth"
	"netguard/internal/computador"
	"netguard/internal/config"
	"netguard/internal/diagnostico"
	"netguard/internal/dispositivos"
	"netguard/internal/monitor"
	"netguard/internal/velocidade"
	"netguard/internal/web"
	"netguard/internal/wifi"
)

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"servir"}
	}
	var err error
	switch args[0] {
	case "servir":
		err = servir()
	case "migrar":
		err = migrar()
	case "usuario":
		err = usuario(args[1:])
	case "versao", "--version", "-v":
		fmt.Println("netguardd", api.Versao)
	default:
		err = fmt.Errorf("comando desconhecido: %s (use servir, migrar, usuario ou versao)", args[0])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "erro:", err)
		var s erroSaida
		if errors.As(err, &s) {
			os.Exit(s.codigo)
		}
		os.Exit(1)
	}
}

// Códigos de saída que o scripts/primeira-vez.sh usa para decidir o que perguntar.
const (
	saidaJaExiste      = 2
	saidaEntradaRuim   = 3
	saidaNaoEncontrado = 4
)

type erroSaida struct {
	codigo int
	err    error
}

func (e erroSaida) Error() string { return e.err.Error() }
func (e erroSaida) Unwrap() error { return e.err }

func carregar(textoNoLog bool) (config.Config, *slog.Logger, error) {
	cfg, err := config.Carregar(os.Getenv)
	if err != nil {
		return cfg, nil, err
	}
	op := &slog.HandlerOptions{Level: cfg.NivelLog}
	var h slog.Handler = slog.NewJSONHandler(os.Stderr, op)
	if textoNoLog {
		h = slog.NewTextHandler(os.Stderr, op)
	}
	return cfg, slog.New(h), nil
}

func servir() error {
	cfg, log, err := carregar(false)
	if err != nil {
		return err
	}
	ctx, parar := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer parar()

	pool, err := armazenamento.Conectar(ctx, cfg.BancoURL, log)
	if err != nil {
		return fmt.Errorf("conectar ao banco: %w", err)
	}
	defer pool.Close()
	feitas, err := armazenamento.Migrar(ctx, pool)
	if err != nil {
		return fmt.Errorf("migrar: %w", err)
	}
	for _, m := range feitas {
		log.Info("migração aplicada", "nome", m)
	}

	repo := armazenamento.NovoRepositorio(pool)
	if n, err := repo.QuantosUsuarios(ctx); err == nil && n == 0 {
		log.Warn("nenhum usuário ainda: crie com 'netguardd usuario criar <nome>'")
	}
	svcAuth := auth.NovoServico(repo, cfg.ValidadeSessao)

	gerente, invalido, err := ajustes.NovoGerente(ctx, repo, ajustes.Padrao(cfg.Intervalo))
	if err != nil {
		return fmt.Errorf("ler configurações: %w", err)
	}
	if invalido != nil {
		log.Warn("configuração salva não vale mais; usando o padrão até salvar de novo", "erro", invalido)
	}

	leitorIface := monitor.NovoLeitorInterface()
	var agendador *velocidade.Agendador // criado depois do monitor: um depende do outro
	var vigiaWifi *wifi.Vigia
	mon := monitor.Novo(monitor.Opcoes{
		Interface:    cfg.Interface,
		Intervalo:    cfg.Intervalo,
		LerInterface: func(nome string) (monitor.EstadoInterface, error) { return leitorIface.Ler(nome) },
		Pinger:       monitor.PingerICMP{Intervalo: 200 * time.Millisecond, Espera: time.Second},
		Conector:     monitor.ConectorTCP{},
		Resolvedor:   monitor.ResolvedorUDP{},
		HTTP:         monitor.NovoClienteHTTP(api.Versao),
		Armazem:      repo,
		Log:          log.With("modulo", "monitor"),
		Versao:       api.Versao,
		Ocupado:      func() bool { return agendador != nil && agendador.Ocupado() },
		Pausado:      func() bool { return vigiaWifi != nil && vigiaWifi.Buscando() },
	})
	gerente.AoMudar(mon.Configurar)

	ifaceAtual := func() (string, string) {
		if e, ok := mon.Estado(); ok && e.Interface.Nome != "" {
			return e.Interface.Nome, e.Interface.Tipo
		}
		// monitor ainda não mediu: pergunta direto ao sistema
		i, _ := leitorIface.Ler(cfg.Interface)
		return i.Nome, i.Tipo
	}
	agendador = velocidade.NovoAgendador(
		velocidade.NovoMedidor("https://speed.cloudflare.com", "NetGuard/"+api.Versao),
		repo, gerente.Velocidade,
		func() bool { e, ok := mon.Estado(); return ok && e.Diagnostico.Estado != diagnostico.Fora },
		func() string { nome, _ := ifaceAtual(); return nome },
		log.With("modulo", "velocidade"),
	)
	if ult, err := repo.UltimoTesteVelocidade(ctx); err == nil {
		agendador.DefinirUltimo(ult)
	}
	coletorWifi := wifi.NovoColetor(wifi.LeitorNmcli{}, repo, ifaceAtual, log.With("modulo", "wifi"))

	vigiaWifi = wifi.NovoVigia(wifi.BuscadorNmcli{}, ifaceAtual, agendador.Ocupado, log.With("modulo", "vizinhanca"))

	saudePC := computador.NovoColetor(computador.NovoLeitor())

	cutucador := monitor.PingerICMP{Espera: 800 * time.Millisecond}
	descoberta := dispositivos.NovoColetor(dispositivos.Opcoes{
		Rede: func() (dispositivos.Rede, bool) {
			e, ok := mon.Estado()
			if !ok || e.ErroInterface != "" {
				return dispositivos.Rede{}, false
			}
			end, err := netip.ParsePrefix(e.Interface.IPv4)
			if err != nil {
				return dispositivos.Rede{}, false
			}
			mac, _ := net.ParseMAC(lerLinha("/sys/class/net/" + e.Interface.Nome + "/address"))
			return dispositivos.Rede{Iface: e.Interface.Nome, Endereco: end, Gateway: e.Interface.Gateway, MAC: mac}, true
		},
		LerVizinhos:    dispositivos.LerVizinhos,
		Cutucar:        func(ctx context.Context, ip netip.Addr) { cutucador.Ping(ctx, ip, 1) },
		NomeReverso:    dispositivos.NomeReverso,
		Fabricantes:    dispositivos.NovaFabricantes("/usr/share/hwdata/oui.txt"),
		VarreduraAtiva: func() bool { return gerente.Atuais().VarreduraAtiva },
		Armazem:        repo,
		Log:            log.With("modulo", "dispositivos"),
	})

	go mon.Rodar(ctx)
	go agendador.Rodar(ctx)
	go coletorWifi.Rodar(ctx)
	go descoberta.Rodar(ctx)
	go vigiaWifi.Rodar(ctx)
	go saudePC.Rodar(ctx)
	go limparSessoes(ctx, svcAuth, log)
	go limparHistorico(ctx, repo, log)

	srv := &http.Server{
		Addr: cfg.Endereco,
		Handler: api.NovoServidor(api.Dependencias{
			Auth:           svcAuth,
			Auditor:        repo,
			Monitor:        mon,
			Consultas:      repo,
			Consultas2B:    repo,
			Ajustes:        gerente,
			Velocidade:     agendador,
			Wifi:           coletorWifi,
			Vizinhanca:     vigiaWifi,
			ConsultasSaude: repo,
			Computador:     saudePC,
			Dispositivos:   repo,
			Descoberta:     descoberta,
			SaudeBanco:     repo.Saude,
			Redes:          cfg.RedesPermitidas,
			CookieSeguro:   cfg.CookieSeguro,
			Log:            log.With("modulo", "api"),
			Web:            web.Handler(),
		}),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       2 * time.Minute,
		MaxHeaderBytes:    32 << 10,
	}
	erros := make(chan error, 1)
	go func() {
		log.Info("netguardd no ar", "endereco", cfg.Endereco, "versao", api.Versao)
		erros <- srv.ListenAndServe()
	}()

	select {
	case err := <-erros:
		return err
	case <-ctx.Done():
	}
	log.Info("encerrando")
	fim, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(fim)
}

func lerLinha(caminho string) string {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func limparSessoes(ctx context.Context, s *auth.Servico, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		if n, err := s.LimparExpiradas(ctx); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn("limpar sessões", "erro", err)
		} else if n > 0 {
			log.Info("sessões expiradas apagadas", "quantas", n)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// limparHistorico apaga as verificações brutas velhas uma vez por hora.
func limparHistorico(ctx context.Context, repo *armazenamento.Repositorio, log *slog.Logger) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		if n, err := repo.LimparAntigos(ctx, time.Now()); err != nil && !errors.Is(err, context.Canceled) {
			log.Warn("limpar histórico", "erro", err)
		} else if n > 0 {
			log.Info("histórico antigo apagado", "linhas", n)
		}
	}
}

func abrirBanco() (*armazenamento.Repositorio, func(), error) {
	cfg, log, err := carregar(true)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pool, err := armazenamento.Conectar(ctx, cfg.BancoURL, log)
	if err != nil {
		return nil, nil, fmt.Errorf("conectar ao banco: %w", err)
	}
	if _, err := armazenamento.Migrar(ctx, pool); err != nil {
		pool.Close()
		return nil, nil, err
	}
	return armazenamento.NovoRepositorio(pool), pool.Close, nil
}

func migrar() error {
	_, fechar, err := abrirBanco()
	if err != nil {
		return err
	}
	fechar()
	fmt.Println("migrações em dia")
	return nil
}

func usuario(args []string) error {
	if len(args) != 2 || (args[0] != "criar" && args[0] != "senha") {
		return errors.New("uso: netguardd usuario criar|senha <nome>")
	}
	nome := args[1]
	if err := auth.ValidarNome(nome); err != nil {
		return erroSaida{saidaEntradaRuim, err}
	}
	senha, err := pedirSenha()
	if err != nil {
		return erroSaida{saidaEntradaRuim, err}
	}
	if err := auth.ValidarSenha(senha); err != nil {
		return erroSaida{saidaEntradaRuim, err}
	}
	repo, fechar, err := abrirBanco()
	if err != nil {
		return err
	}
	defer fechar()
	svc := auth.NovoServico(repo, time.Hour)
	ctx := context.Background()

	acao := "usuario_criado"
	if args[0] == "criar" {
		err = svc.CriarUsuario(ctx, nome, senha)
		if errors.Is(err, auth.ErrJaExiste) {
			return erroSaida{saidaJaExiste, fmt.Errorf("usuário %s já existe (para trocar a senha: netguardd usuario senha %s)", nome, nome)}
		}
	} else {
		acao = "senha_trocada"
		err = svc.TrocarSenha(ctx, nome, senha)
		if errors.Is(err, auth.ErrNaoEncontrado) {
			return erroSaida{saidaNaoEncontrado, fmt.Errorf("usuário %s não existe", nome)}
		}
	}
	if err != nil {
		return err
	}
	repo.Auditar(ctx, armazenamento.Auditoria{Usuario: "terminal", Acao: acao, Alvo: nome})
	fmt.Println("pronto:", acao, nome)
	return nil
}

// pedirSenha lê sem mostrar na tela; fora de um terminal, lê uma linha da entrada.
func pedirSenha() (string, error) {
	fd := int(os.Stdin.Fd())
	if !term.IsTerminal(fd) {
		linha, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && linha == "" {
			return "", errors.New("senha não informada")
		}
		return strings.TrimRight(linha, "\r\n"), nil
	}
	fmt.Fprint(os.Stderr, "Senha (10 a 128 caracteres): ")
	a, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	fmt.Fprint(os.Stderr, "Repita a senha: ")
	b, err := term.ReadPassword(fd)
	fmt.Fprintln(os.Stderr)
	if err != nil {
		return "", err
	}
	if string(a) != string(b) {
		return "", errors.New("as senhas não conferem")
	}
	return string(a), nil
}
