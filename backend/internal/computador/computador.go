// Package computador lê a saúde da máquina onde o NetGuard roda: memória,
// pressão de memória/CPU/disco, carga, espaço em disco, temperatura e tempo
// ligado. Tudo vem de arquivos que o kernel já mantém (/proc e /sys): ler custa
// quase nada e não estressa a máquina. Nada é gravado no banco; a última hora
// fica na memória.
package computador

import (
	"bufio"
	"context"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

type Disco struct {
	Ponto   string  `json:"ponto"`
	TotalGB float64 `json:"total_gb"`
	LivreGB float64 `json:"livre_gb"`
	UsoPct  float64 `json:"uso_pct"`
}

// Leitura é uma foto da máquina.
type Leitura struct {
	Em              time.Time `json:"em"`
	MemTotalMB      int       `json:"mem_total_mb"`
	MemDisponivelMB int       `json:"mem_disponivel_mb"`
	SwapTotalMB     int       `json:"swap_total_mb"`
	SwapUsadoMB     int       `json:"swap_usado_mb"`
	// Pressão (PSI): % do tempo em que algum programa ficou esperando por
	// memória, CPU ou disco, na média dos últimos 5 minutos. 0 = nunca.
	PressaoMem      *float64 `json:"pressao_mem"`
	PressaoMemTotal *float64 `json:"pressao_mem_total"` // "full": todos travados esperando memória
	PressaoCPU      *float64 `json:"pressao_cpu"`
	PressaoDisco    *float64 `json:"pressao_disco"`
	Carga5          float64  `json:"carga_5"` // média de 5 min de programas querendo CPU
	Nucleos         int      `json:"nucleos"`
	Discos          []Disco  `json:"discos"`
	TempCPU         *float64 `json:"temp_cpu"` // °C
	TempSensor      string   `json:"temp_sensor,omitempty"`
	LigadoS         float64  `json:"ligado_s"`
}

// Leitor lê de uma raiz (/ de verdade; uma pasta falsa nos testes).
type Leitor struct {
	Raiz   string
	Pontos []string // pontos de montagem a olhar (ex.: /, /home)
	statfs func(caminho string) (total, livre uint64, err error)
}

func NovoLeitor() *Leitor {
	return &Leitor{Raiz: "/", Pontos: []string{"/", "/home"}, statfs: statfsReal}
}

func statfsReal(caminho string) (uint64, uint64, error) {
	var s syscall.Statfs_t
	if err := syscall.Statfs(caminho, &s); err != nil {
		return 0, 0, err
	}
	return s.Blocks * uint64(s.Bsize), s.Bavail * uint64(s.Bsize), nil
}

func (l *Leitor) arq(p string) string { return filepath.Join(l.Raiz, p) }

func (l *Leitor) Ler() Leitura {
	r := Leitura{Em: time.Now()}
	mem := lerChaves(l.arq("proc/meminfo"))
	r.MemTotalMB = mem["MemTotal"] / 1024
	r.MemDisponivelMB = mem["MemAvailable"] / 1024
	r.SwapTotalMB = mem["SwapTotal"] / 1024
	r.SwapUsadoMB = (mem["SwapTotal"] - mem["SwapFree"]) / 1024

	r.PressaoMem, r.PressaoMemTotal = psi(l.arq("proc/pressure/memory"))
	r.PressaoCPU, _ = psi(l.arq("proc/pressure/cpu"))
	r.PressaoDisco, _ = psi(l.arq("proc/pressure/io"))

	if b, err := os.ReadFile(l.arq("proc/loadavg")); err == nil {
		if c := strings.Fields(string(b)); len(c) >= 2 {
			r.Carga5, _ = strconv.ParseFloat(c[1], 64)
		}
	}
	r.Nucleos = nucleos(l.arq("proc/cpuinfo"))

	vistos := map[string]bool{}
	for _, p := range l.Pontos {
		total, livre, err := l.statfs(filepath.Join(l.Raiz, p))
		if err != nil || total == 0 {
			continue
		}
		chave := strconv.FormatUint(total, 10) + "/" + strconv.FormatUint(livre/(1<<30), 10)
		if vistos[chave] { // /home no mesmo disco que /: mostra uma vez
			continue
		}
		vistos[chave] = true
		gb := func(b uint64) float64 { return float64(int64(float64(b)/(1<<30)*10+0.5)) / 10 }
		r.Discos = append(r.Discos, Disco{Ponto: p, TotalGB: gb(total), LivreGB: gb(livre),
			UsoPct: float64(int64(float64(total-livre)/float64(total)*1000+0.5)) / 10})
	}

	r.TempCPU, r.TempSensor = temperatura(l.arq("sys/class/hwmon"), l.arq("sys/class/thermal"))
	if b, err := os.ReadFile(l.arq("proc/uptime")); err == nil {
		if c := strings.Fields(string(b)); len(c) > 0 {
			r.LigadoS, _ = strconv.ParseFloat(c[0], 64)
		}
	}
	return r
}

// lerChaves lê "Chave:   123 kB" (meminfo).
func lerChaves(caminho string) map[string]int {
	m := map[string]int{}
	f, err := os.Open(caminho)
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		if c := strings.Fields(v); len(c) > 0 {
			n, _ := strconv.Atoi(c[0])
			m[k] = n
		}
	}
	return m
}

// psi lê avg300 das linhas "some" e "full" de /proc/pressure/*.
func psi(caminho string) (some, full *float64) {
	b, err := os.ReadFile(caminho)
	if err != nil {
		return nil, nil // kernel sem PSI
	}
	for _, linha := range strings.Split(string(b), "\n") {
		c := strings.Fields(linha)
		if len(c) == 0 {
			continue
		}
		for _, campo := range c[1:] {
			if v, ok := strings.CutPrefix(campo, "avg300="); ok {
				x, err := strconv.ParseFloat(v, 64)
				if err != nil {
					continue
				}
				switch c[0] {
				case "some":
					some = &x
				case "full":
					full = &x
				}
			}
		}
	}
	return some, full
}

func nucleos(cpuinfo string) int {
	f, err := os.Open(cpuinfo)
	if err != nil {
		return 1
	}
	defer f.Close()
	n := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if strings.HasPrefix(sc.Text(), "processor") {
			n++
		}
	}
	return max(1, n)
}

// Sensores de CPU conhecidos, do mais confiável ao menos. Outros (como os do
// Mac, que às vezes mandam valores absurdos) são ignorados.
var sensoresCPU = []string{"coretemp", "k10temp", "zenpower", "cpu_thermal", "cpu-thermal", "soc_thermal", "acpitz"}

// temperatura pega a maior leitura do sensor de CPU mais confiável que existir.
func temperatura(hwmon, thermal string) (*float64, string) {
	achados := map[string]float64{}
	for _, d := range listar(hwmon) {
		nome := lerTexto(filepath.Join(d, "name"))
		maior := -1.0
		for _, arq := range glob(filepath.Join(d, "temp*_input")) {
			if v, ok := grausDe(arq); ok {
				maior = max(maior, v)
			}
		}
		if maior >= 0 {
			achados[nome] = max(achados[nome], maior)
		}
	}
	for _, d := range listar(thermal) { // Raspberry Pi e afins
		if !strings.Contains(filepath.Base(d), "thermal_zone") {
			continue
		}
		if v, ok := grausDe(filepath.Join(d, "temp")); ok {
			nome := lerTexto(filepath.Join(d, "type"))
			achados[nome] = max(achados[nome], v)
		}
	}
	for _, s := range sensoresCPU {
		if v, ok := achados[s]; ok {
			return &v, s
		}
	}
	return nil, ""
}

// grausDe lê milésimos de grau; fora de 1–125 °C é sensor quebrado.
func grausDe(arq string) (float64, bool) {
	n, err := strconv.Atoi(lerTexto(arq))
	if err != nil {
		return 0, false
	}
	v := float64(n) / 1000
	return v, v > 1 && v < 125
}

func lerTexto(p string) string {
	b, _ := os.ReadFile(p)
	return strings.TrimSpace(string(b))
}

func listar(dir string) []string {
	es, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var r []string
	for _, e := range es {
		r = append(r, filepath.Join(dir, e.Name()))
	}
	sort.Strings(r)
	return r
}

func glob(padrao string) []string {
	m, _ := filepath.Glob(padrao)
	return m
}

// Resumo é o que vai para a tela: a leitura atual e o pior da última hora.
type Resumo struct {
	Agora        Leitura  `json:"agora"`
	TempMaxHora  *float64 `json:"temp_max_hora"`
	MemMinHoraMB int      `json:"mem_min_hora_mb"`
	Leituras     int      `json:"leituras"`
}

// Coletor lê a máquina a cada minuto e guarda a última hora na memória.
type Coletor struct {
	leitor *Leitor
	mu     sync.Mutex
	hora   []Leitura
}

func NovoColetor(l *Leitor) *Coletor { return &Coletor{leitor: l} }

func (c *Coletor) Rodar(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		c.coletar()
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Coletor) coletar() {
	l := c.leitor.Ler()
	c.mu.Lock()
	c.hora = append(c.hora, l)
	if len(c.hora) > 60 {
		c.hora = c.hora[len(c.hora)-60:]
	}
	c.mu.Unlock()
}

func (c *Coletor) Resumo() (*Resumo, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.hora) == 0 {
		return nil, false
	}
	r := &Resumo{Agora: c.hora[len(c.hora)-1], Leituras: len(c.hora), MemMinHoraMB: c.hora[0].MemDisponivelMB}
	for _, l := range c.hora {
		if l.TempCPU != nil && (r.TempMaxHora == nil || *l.TempCPU > *r.TempMaxHora) {
			t := *l.TempCPU
			r.TempMaxHora = &t
		}
		r.MemMinHoraMB = min(r.MemMinHoraMB, l.MemDisponivelMB)
	}
	return r, true
}
