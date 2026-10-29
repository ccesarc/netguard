package ajustes

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPadraoValido(t *testing.T) {
	a := Padrao(10 * time.Second)
	if err := a.Validar(); err != nil {
		t.Fatalf("padrão inválido: %v", err)
	}
	if a.ConsumoMensalMB() != 3*35*30 {
		t.Fatalf("consumo: %d", a.ConsumoMensalMB())
	}
	a.Velocidade.Ativo = false
	if a.ConsumoMensalMB() != 0 {
		t.Fatal("desligado não consome")
	}
}

func TestValidacoes(t *testing.T) {
	casos := map[string]func(*Ajustes){
		"intervalo curto":  func(a *Ajustes) { a.IntervaloS = 1 },
		"sem externos":     func(a *Ajustes) { a.Externos = nil },
		"externo inválido": func(a *Ajustes) { a.Externos = []string{"um.ip"} },
		"externo local":    func(a *Ajustes) { a.Externos = []string{"10.0.0.1"} },
		"externo repetido": func(a *Ajustes) { a.Externos = []string{"1.1.1.1", "1.1.1.1"} },
		"externo ipv6":     func(a *Ajustes) { a.Externos = []string{"2606:4700::1111"} },
		"dns inválido":     func(a *Ajustes) { a.DNSComparacao = "dns.google" },
		"url sem esquema":  func(a *Ajustes) { a.URLs = []string{"google.com"} },
		"url ftp":          func(a *Ajustes) { a.URLs = []string{"ftp://x.com"} },
		"url com senha":    func(a *Ajustes) { a.URLs = []string{"https://eu:segredo@x.com"} },
		"url javascript":   func(a *Ajustes) { a.URLs = []string{"javascript:alert(1)"} },
		"muitas urls": func(a *Ajustes) {
			a.URLs = []string{"https://a.com", "https://b.com", "https://c.com", "https://d.com", "https://e.com", "https://f.com"}
		},
		"latência":            func(a *Ajustes) { a.LimiteLatenciaMS = 5 },
		"perda":               func(a *Ajustes) { a.LimitePerda = 0.9 },
		"velocidade vezes":    func(a *Ajustes) { a.Velocidade.VezesPorDia = 48 },
		"velocidade download": func(a *Ajustes) { a.Velocidade.MBDownload = 1000 },
		"plano negativo":      func(a *Ajustes) { a.PlanoDownloadMbps = -1 },
		"plano absurdo":       func(a *Ajustes) { a.PlanoUploadMbps = 50000 },
	}
	for nome, mudar := range casos {
		a := Padrao(10 * time.Second)
		mudar(&a)
		if a.Validar() == nil {
			t.Errorf("%s: esperava erro", nome)
		}
	}
}

func TestTodosOsErrosDeUmaVez(t *testing.T) {
	a := Padrao(10 * time.Second)
	a.IntervaloS = 0
	a.LimitePerda = 2
	err := a.Validar()
	if err == nil || !strings.Contains(err.Error(), "intervalo") || !strings.Contains(err.Error(), "perda") {
		t.Fatalf("deveria listar os dois problemas: %v", err)
	}
}

func TestSemSitesPode(t *testing.T) {
	a := Padrao(10 * time.Second)
	a.URLs = nil
	if err := a.Validar(); err != nil {
		t.Fatalf("lista de sites vazia é permitida: %v", err)
	}
}

type armazemFalso struct {
	salvo *Ajustes
	vezes int
}

func (a *armazemFalso) LerAjustes(context.Context) (Ajustes, bool, error) {
	if a.salvo == nil {
		return Ajustes{}, false, nil
	}
	return *a.salvo, true, nil
}

func (a *armazemFalso) SalvarAjustes(_ context.Context, x Ajustes, _ string) error {
	a.salvo = &x
	a.vezes++
	return nil
}

func TestGerente(t *testing.T) {
	ctx := context.Background()
	arm := &armazemFalso{}
	g, inv, err := NovoGerente(ctx, arm, Padrao(10*time.Second))
	if err != nil || inv != nil || g.Atuais().IntervaloS != 10 {
		t.Fatalf("novo: %v %v %+v", err, inv, g.Atuais())
	}
	var recebidos []int
	g.AoMudar(func(a Ajustes) { recebidos = append(recebidos, a.IntervaloS) })

	a := g.Atuais()
	a.IntervaloS = 1
	if err := g.Salvar(ctx, a, "dono"); err == nil || arm.vezes != 0 {
		t.Fatal("inválido não pode ser gravado")
	}
	a.IntervaloS = 20
	if err := g.Salvar(ctx, a, "dono"); err != nil {
		t.Fatal(err)
	}
	if g.Atuais().IntervaloS != 20 || len(recebidos) != 2 || recebidos[1] != 20 {
		t.Fatalf("aplicação: atual=%d avisos=%v", g.Atuais().IntervaloS, recebidos)
	}

	// cópia: mexer no que Atuais devolveu não muda o gerente
	c := g.Atuais()
	c.Externos[0] = "9.9.9.9"
	if g.Atuais().Externos[0] != "1.1.1.1" {
		t.Fatal("Atuais expôs a lista interna")
	}

	// salvo inválido (regra mudou): usa o padrão e avisa
	ruim := Padrao(10 * time.Second)
	ruim.IntervaloS = 2
	g, inv, _ = NovoGerente(ctx, &armazemFalso{salvo: &ruim}, Padrao(10*time.Second))
	if inv == nil || g.Atuais().IntervaloS != 10 {
		t.Fatalf("salvo inválido: %v %+v", inv, g.Atuais())
	}
}
