# Medições feitas à mão

O que já dá para saber **só com este PC**, medido antes de existir código. Serve de base para o
que o NetGuard vai automatizar e para conferir se ele mede certo.

## 2026-10-29 — primeira varredura (com ok do dono)

Feito só com o que já estava instalado: `ping`, `ip neigh`, `dig`, `curl`, `nmcli`, `upnpc` e a
tabela de fabricantes do sistema (`/usr/share/hwdata/oui.txt`).

### Wi-Fi deste PC

| Item | Valor |
|---|---|
| Rede | `MINHA_REDE_5G` (WPA2) |
| Faixa / canal | 5 GHz, canal 48 (5240 MHz) |
| Sinal | 79% |
| Velocidade da ligação com o roteador | 1170 Mbit/s |
| Redes vizinhas vistas | nenhuma além da própria (a busca do `nmcli` pode estar limitada pelo driver `wl`) |

### Aparelhos

Só escutando (sem enviar nada), o PC conhecia **2**. Depois de um ping em cada endereço de
10.0.0.1 a 10.0.0.254, apareceram **8 + este PC**:

| IP | MAC | Fabricante | Nome (pelo roteador) | Palpite |
|---|---|---|---|---|
| 10.0.0.1 | 54:6c:ac:12:34:43 | Intelbras | meuintelbras.local | roteador |
| 10.0.0.2 | 56:e2:c7:12:34:ac | MAC aleatório | Celular-Android.local | celular Xiaomi |
| 10.0.0.28 | 66:a6:27:12:34:7f | MAC aleatório | — | celular/tablet? |
| 10.0.0.30 | e6:09:58:12:34:33 | MAC aleatório | Notebook.local | Mac mini |
| 10.0.0.133 | f8:01:b4:12:34:b4 | LG Electronics | LGwebOSTV.local | TV LG |
| 10.0.0.168 | 74:ec:b2:12:34:ad | Amazon | — | Echo/Fire TV? |
| 10.0.0.175 | b6:67:26:12:34:ea | MAC aleatório | — | celular/tablet? |
| 10.0.0.196 | 70:70:aa:12:34:34 | Amazon | — | Echo/Fire TV? |
| 10.0.0.209 | 14:bd:61:12:34:c4 | — | — | este PC |

A TV da TCL (projeto Divullg TV) não apareceu; provavelmente estava desligada.

**O que isso ensina:**
- A varredura ativa é necessária: sem ela a lista fica quase vazia.
- O **DNS reverso no roteador** (`dig -x <ip> @10.0.0.1`) dá o nome que o aparelho informou no
  DHCP. Funcionou para 4 de 8. É a melhor fonte de nome sem senha do roteador.
- 4 de 8 aparelhos usam **MAC aleatório**. Fabricante não sai deles; o nome do DHCP é o que
  identifica.
- **mDNS** não respondeu deste PC (avahi sem o serviço rodando, `dig` direto no 224.0.0.251 sem
  resposta). Fica para investigar; pode render nomes dos aparelhos sem nome.

### Roteador

| Item | Valor |
|---|---|
| Marca | Intelbras (MAC e página web) |
| Painel web | `http://10.0.0.1/` responde (título "Intelbras"); precisa da senha para ver mais |
| UPnP | **desligado** ("No IGD UPnP Device found") → sem total de tráfego da casa sem senha |

Se o dono tiver a senha do painel: muitos Intelbras mostram a lista de clientes com nome e às
vezes consumo, e alguns têm SNMP. Ver o modelo exato no painel ou na etiqueta.

### Internet

| Medida | Valor |
|---|---|
| Latência até o roteador | média 6,8 ms (2,0–19,4), 0% de perda — o pico de 19 ms é o Wi-Fi |
| Latência até 1.1.1.1 (Cloudflare) | média 48,8 ms (44,7–54,3), 0% de perda |
| Latência até 8.8.8.8 (Google) | média 40,6 ms (36,0–46,2), 0% de perda |
| Download | ~242 Mbit/s (50 MB em 1,65 s, `speed.cloudflare.com`) |
| Upload | ~85 Mbit/s (10 MB em 0,94 s) |

Teste curto, um só, pelo Wi-Fi: serve de ordem de grandeza, não de laudo. O NetGuard deve repetir
em horários diferentes e guardar o histórico. O `/meta` da Cloudflare (provedor e IP público)
voltou vazio.
