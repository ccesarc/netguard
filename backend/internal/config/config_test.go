package config

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func ambiente(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestPadroes(t *testing.T) {
	c, err := Carregar(ambiente(map[string]string{"NETGUARD_BANCO_URL": "postgres://x"}))
	if err != nil {
		t.Fatal(err)
	}
	if c.Endereco != ":8095" || c.Intervalo != 10*time.Second || c.NivelLog != slog.LevelInfo {
		t.Fatalf("padrões errados: %+v", c)
	}
	if len(c.RedesPermitidas) != len(RedesPrivadas) {
		t.Fatalf("redes padrão: %v", c.RedesPermitidas)
	}
	if c.CookieSeguro {
		t.Fatal("cookie seguro deveria vir desligado (painel em http na rede local)")
	}
}

func TestBancoPorArquivo(t *testing.T) {
	arq := filepath.Join(t.TempDir(), "url")
	if err := os.WriteFile(arq, []byte("postgres://arquivo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	c, err := Carregar(ambiente(map[string]string{"NETGUARD_BANCO_URL_ARQUIVO": arq}))
	if err != nil {
		t.Fatal(err)
	}
	if c.BancoURL != "postgres://arquivo" {
		t.Fatalf("url: %q", c.BancoURL)
	}
}

func TestErros(t *testing.T) {
	casos := map[string]map[string]string{
		"sem banco":           {},
		"endereço":            {"NETGUARD_BANCO_URL": "x", "NETGUARD_ENDERECO": "8095"},
		"interface":           {"NETGUARD_BANCO_URL": "x", "NETGUARD_INTERFACE": "wlan0; rm -rf /"},
		"intervalo curto":     {"NETGUARD_BANCO_URL": "x", "NETGUARD_INTERVALO": "500ms"},
		"intervalo inválido":  {"NETGUARD_BANCO_URL": "x", "NETGUARD_INTERVALO": "dez"},
		"log":                 {"NETGUARD_BANCO_URL": "x", "NETGUARD_LOG": "tudo"},
		"redes":               {"NETGUARD_BANCO_URL": "x", "NETGUARD_REDES": "10.0.0.0/33"},
		"cookie":              {"NETGUARD_BANCO_URL": "x", "NETGUARD_COOKIE_SEGURO": "talvez"},
		"arquivo inexistente": {"NETGUARD_BANCO_URL_ARQUIVO": "/nao/existe"},
	}
	for nome, env := range casos {
		if _, err := Carregar(ambiente(env)); err == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}
