package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Parâmetros do argon2id (mínimo recomendado pela OWASP: 19 MiB, 2 passadas).
// Leves o bastante para um Raspberry Pi.
const (
	argonTempo   = 2
	argonMemoria = 19 * 1024 // KiB
	argonThreads = 1
	argonTamanho = 32
	tamanhoSalt  = 16
)

// HashSenha gera o hash no formato PHC: $argon2id$v=19$m=...,t=...,p=...$salt$hash
func HashSenha(senha string) (string, error) {
	salt := make([]byte, tamanhoSalt)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(senha), salt, argonTempo, argonMemoria, argonThreads, argonTamanho)
	return fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version, argonMemoria, argonTempo, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(h)), nil
}

// VerificarSenha compara em tempo constante. Hash malformado = senha errada.
func VerificarSenha(senha, codificado string) bool {
	partes := strings.Split(codificado, "$")
	if len(partes) != 6 || partes[1] != "argon2id" {
		return false
	}
	var versao int
	if _, err := fmt.Sscanf(partes[2], "v=%d", &versao); err != nil || versao != argon2.Version {
		return false
	}
	var m, t uint32
	var p uint8
	if _, err := fmt.Sscanf(partes[3], "m=%d,t=%d,p=%d", &m, &t, &p); err != nil {
		return false
	}
	// Limites para um hash adulterado no banco não travar o serviço.
	if m == 0 || m > 256*1024 || t == 0 || t > 10 || p == 0 || p > 8 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(partes[4])
	if err != nil {
		return false
	}
	esperado, err := base64.RawStdEncoding.DecodeString(partes[5])
	if err != nil || len(esperado) == 0 || len(esperado) > 64 {
		return false
	}
	h := argon2.IDKey([]byte(senha), salt, t, m, p, uint32(len(esperado)))
	return subtle.ConstantTimeCompare(h, esperado) == 1
}
