// Package web serve o frontend compilado, embutido no binário.
// O Vite compila para dist/app (fora do git); dist/.gitkeep só existe para o embed
// funcionar antes da primeira compilação.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var arquivos embed.FS

// Handler serve os arquivos e devolve o index.html para as rotas do Vue.
func Handler() http.Handler {
	app, _ := fs.Sub(arquivos, "dist/app")
	return handlerDe(app)
}

func handlerDe(app fs.FS) http.Handler {
	arquivosHTTP := http.FileServerFS(app)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "método não permitido", http.StatusMethodNotAllowed)
			return
		}
		nome := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if nome != "" && nome != "index.html" {
			if f, err := app.Open(nome); err == nil {
				f.Close()
				if strings.HasPrefix(nome, "assets/") {
					// Os nomes levam hash do conteúdo: podem ficar em cache para sempre.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				arquivosHTTP.ServeHTTP(w, r)
				return
			}
			if path.Ext(nome) != "" {
				http.NotFound(w, r)
				return
			}
		}
		indice, err := fs.ReadFile(app, "index.html")
		if err != nil {
			http.Error(w, "frontend não compilado: rode npm run build em frontend/", http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(indice)
	})
}
