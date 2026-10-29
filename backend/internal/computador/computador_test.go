package computador

import (
	"os"
	"path/filepath"
	"testing"
)

// raiz falsa com os arquivos como estavam neste PC em 2026-10-29.
func raizFalsa(t *testing.T) string {
	t.Helper()
	r := t.TempDir()
	escrever := func(p, conteudo string) {
		os.MkdirAll(filepath.Dir(filepath.Join(r, p)), 0o755)
		os.WriteFile(filepath.Join(r, p), []byte(conteudo), 0o644)
	}
	escrever("proc/meminfo", "MemTotal:        7996248 kB\nMemFree:         2331608 kB\nMemAvailable:    5656716 kB\nSwapTotal:       7995388 kB\nSwapFree:        7564916 kB\n")
	escrever("proc/pressure/memory", "some avg10=0.00 avg60=0.00 avg300=0.12 total=4176000\nfull avg10=0.00 avg60=0.00 avg300=0.05 total=1000\n")
	escrever("proc/pressure/cpu", "some avg10=0.03 avg60=0.30 avg300=0.40 total=161373468\n")
	escrever("proc/pressure/io", "some avg10=0.01 avg60=0.21 avg300=0.39 total=78890434\nfull avg10=0 avg60=0 avg300=0.1 total=1\n")
	escrever("proc/loadavg", "0.41 0.35 0.31 1/555 134096\n")
	escrever("proc/cpuinfo", "processor\t: 0\nprocessor\t: 1\nprocessor\t: 2\nprocessor\t: 3\n")
	escrever("proc/uptime", "17280.50 60000.00\n")
	// sensor do Mac com valor absurdo + coretemp de verdade
	escrever("sys/class/hwmon/hwmon0/name", "macsmc_hwmon\n")
	escrever("sys/class/hwmon/hwmon0/temp1_input", "10752000\n")
	escrever("sys/class/hwmon/hwmon1/name", "coretemp\n")
	escrever("sys/class/hwmon/hwmon1/temp1_input", "56000\n")
	escrever("sys/class/hwmon/hwmon1/temp2_input", "53000\n")
	return r
}

func TestLer(t *testing.T) {
	l := &Leitor{Raiz: raizFalsa(t), Pontos: []string{"/", "/home"}, statfs: func(p string) (uint64, uint64, error) {
		return 444 << 30, 280 << 30, nil // / e /home no mesmo disco
	}}
	r := l.Ler()
	if r.MemTotalMB != 7808 || r.MemDisponivelMB != 5524 || r.SwapUsadoMB != 420 {
		t.Fatalf("memória: %+v", r)
	}
	if *r.PressaoMem != 0.12 || *r.PressaoMemTotal != 0.05 || *r.PressaoCPU != 0.4 || *r.PressaoDisco != 0.39 {
		t.Fatalf("pressão: %v %v %v %v", *r.PressaoMem, *r.PressaoMemTotal, *r.PressaoCPU, *r.PressaoDisco)
	}
	if r.Carga5 != 0.35 || r.Nucleos != 4 || r.LigadoS != 17280.5 {
		t.Fatalf("carga/núcleos/ligado: %+v", r)
	}
	if len(r.Discos) != 1 || r.Discos[0].UsoPct != 36.9 || r.Discos[0].LivreGB != 280 {
		t.Fatalf("disco (mesmo disco aparece uma vez): %+v", r.Discos)
	}
	if r.TempCPU == nil || *r.TempCPU != 56 || r.TempSensor != "coretemp" {
		t.Fatalf("temperatura (ignorar o sensor absurdo): %v %s", r.TempCPU, r.TempSensor)
	}
}

func TestSemPSIESemSensor(t *testing.T) {
	r := (&Leitor{Raiz: t.TempDir(), statfs: func(string) (uint64, uint64, error) { return 0, 0, os.ErrNotExist }}).Ler()
	if r.PressaoMem != nil || r.TempCPU != nil || len(r.Discos) != 0 || r.Nucleos != 1 {
		t.Fatalf("sem arquivos não pode inventar: %+v", r)
	}
}

func TestTemperaturaDoPi(t *testing.T) {
	r := t.TempDir()
	os.MkdirAll(filepath.Join(r, "sys/class/thermal/thermal_zone0"), 0o755)
	os.WriteFile(filepath.Join(r, "sys/class/thermal/thermal_zone0/type"), []byte("cpu-thermal\n"), 0o644)
	os.WriteFile(filepath.Join(r, "sys/class/thermal/thermal_zone0/temp"), []byte("61200\n"), 0o644)
	v, s := temperatura(filepath.Join(r, "sys/class/hwmon"), filepath.Join(r, "sys/class/thermal"))
	if v == nil || *v != 61.2 || s != "cpu-thermal" {
		t.Fatalf("Pi: %v %s", v, s)
	}
}

func TestColetorGuardaOPiorDaHora(t *testing.T) {
	raiz := raizFalsa(t)
	l := &Leitor{Raiz: raiz, Pontos: []string{"/"}, statfs: func(string) (uint64, uint64, error) { return 100 << 30, 50 << 30, nil }}
	c := NovoColetor(l)
	if _, ok := c.Resumo(); ok {
		t.Fatal("antes de ler não tem resumo")
	}
	c.coletar()
	os.WriteFile(filepath.Join(raiz, "sys/class/hwmon/hwmon1/temp1_input"), []byte("81000\n"), 0o644)
	os.WriteFile(filepath.Join(raiz, "proc/meminfo"), []byte("MemTotal: 7996248 kB\nMemAvailable: 409600 kB\n"), 0o644)
	c.coletar()
	os.WriteFile(filepath.Join(raiz, "sys/class/hwmon/hwmon1/temp1_input"), []byte("50000\n"), 0o644)
	c.coletar()
	r, _ := c.Resumo()
	if *r.TempMaxHora != 81 || r.MemMinHoraMB != 400 || r.Leituras != 3 || *r.Agora.TempCPU != 53 {
		t.Fatalf("resumo: max=%v memMin=%d leituras=%d agora=%v", *r.TempMaxHora, r.MemMinHoraMB, r.Leituras, *r.Agora.TempCPU)
	}
}

// Leitura real deste PC.
func TestLeituraReal(t *testing.T) {
	r := NovoLeitor().Ler()
	if r.MemTotalMB <= 0 || r.Nucleos < 1 || len(r.Discos) == 0 {
		t.Fatalf("leitura real: %+v", r)
	}
	t.Logf("memória %d/%d MB · carga %.2f/%d · discos %+v · temp %v (%s)", r.MemDisponivelMB, r.MemTotalMB, r.Carga5, r.Nucleos, r.Discos, deref(r.TempCPU), r.TempSensor)
}

func deref(p *float64) float64 {
	if p == nil {
		return -1
	}
	return *p
}
