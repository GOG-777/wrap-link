package handler

import (
	"net/http"

	"github.com/GOG-777/wrap-link/pkg/wrap"
)

// Handler is the Vercel serverless entrypoint for wrap.thgenius.xyz.
func Handler(w http.ResponseWriter, r *http.Request) {
	wrap.DefaultServer().ServeHTTP(w, r)
}
