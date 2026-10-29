// Package config lê a configuração do serviço a partir de variáveis de ambiente.
//
// Só o que precisa existir antes do banco fica aqui; alvos, limites e o resto
// da configuração do monitor vão para o banco nas próximas fases.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Endereco        string
	BancoURL        string
	Interface       string // vazio = a interface da rota padrão
	NivelLog        slog.Level
	Intervalo       time.Duration
	RedesPermitidas []netip.Prefix
	CookieSeguro    bool
	ValidadeSessao  time.Duration
}

// RedesPrivadas são as origens aceitas por padrão: a própria máquina e redes locais.
var RedesPrivadas = []string{
	"127.0.0.0/8", "::1/128",
	"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16",
	"fc00::/7", "fe80::/10",
}

var nomeInterface = regexp.MustCompile(`^[a-zA-Z0-9_.:-]{1,15}$`)

// Carregar monta a configuração. getenv é os.Getenv fora dos testes.
func Carregar(getenv func(string) string) (Config, error) {
	c := Config{
		Endereco:       ":8095",
		NivelLog:       slog.LevelInfo,
		Intervalo:      10 * time.Second,
		ValidadeSessao: 7 * 24 * time.Hour,
	}

	if v := getenv("NETGUARD_ENDERECO"); v != "" {
		c.Endereco = v
	}
	if _, _, err := net.SplitHostPort(c.Endereco); err != nil {
		return c, fmt.Errorf("NETGUARD_ENDERECO inválido (%q): %w", c.Endereco, err)
	}

	url, err := lerBancoURL(getenv)
	if err != nil {
		return c, err
	}
	c.BancoURL = url

	if v := getenv("NETGUARD_INTERFACE"); v != "" {
		if !nomeInterface.MatchString(v) {
			return c, fmt.Errorf("NETGUARD_INTERFACE inválido: %q", v)
		}
		c.Interface = v
	}

	if v := getenv("NETGUARD_LOG"); v != "" {
		switch strings.ToLower(v) {
		case "debug":
			c.NivelLog = slog.LevelDebug
		case "info":
			c.NivelLog = slog.LevelInfo
		case "warn", "aviso":
			c.NivelLog = slog.LevelWarn
		case "error", "erro":
			c.NivelLog = slog.LevelError
		default:
			return c, fmt.Errorf("NETGUARD_LOG inválido: %q (use debug, info, warn ou error)", v)
		}
	}

	if v := getenv("NETGUARD_INTERVALO"); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil {
			return c, fmt.Errorf("NETGUARD_INTERVALO inválido: %w", err)
		}
		if d < 2*time.Second || d > 10*time.Minute {
			return c, fmt.Errorf("NETGUARD_INTERVALO fora do limite (2s a 10m): %s", d)
		}
		c.Intervalo = d
	}

	redes := RedesPrivadas
	if v := getenv("NETGUARD_REDES"); v != "" {
		redes = strings.Split(v, ",")
	}
	for _, r := range redes {
		p, err := netip.ParsePrefix(strings.TrimSpace(r))
		if err != nil {
			return c, fmt.Errorf("NETGUARD_REDES inválido (%q): %w", r, err)
		}
		c.RedesPermitidas = append(c.RedesPermitidas, p.Masked())
	}

	if v := getenv("NETGUARD_COOKIE_SEGURO"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("NETGUARD_COOKIE_SEGURO inválido: %q", v)
		}
		c.CookieSeguro = b
	}

	return c, nil
}

// lerBancoURL aceita a URL direto ou um arquivo com ela (preferido: não fica no ambiente).
func lerBancoURL(getenv func(string) string) (string, error) {
	if v := getenv("NETGUARD_BANCO_URL"); v != "" {
		return v, nil
	}
	arquivo := getenv("NETGUARD_BANCO_URL_ARQUIVO")
	if arquivo == "" {
		return "", errors.New("defina NETGUARD_BANCO_URL ou NETGUARD_BANCO_URL_ARQUIVO")
	}
	arquivo, err := expandirCasa(arquivo)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(arquivo)
	if err != nil {
		return "", fmt.Errorf("ler NETGUARD_BANCO_URL_ARQUIVO: %w", err)
	}
	url := strings.TrimSpace(string(b))
	if url == "" {
		return "", fmt.Errorf("arquivo %s está vazio", arquivo)
	}
	return url, nil
}

func expandirCasa(caminho string) (string, error) {
	if !strings.HasPrefix(caminho, "~/") {
		return caminho, nil
	}
	casa, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(casa, caminho[2:]), nil
}
