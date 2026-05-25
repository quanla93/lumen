// Package main is the entry point for the Lumen hub binary.
//
// Phase 1.1/1.2 status: skeleton only — wires up a chi router but does not
// register any routes yet. Phase 1.3 will add /healthz; subsequent phases add
// ingest, stream, auth, etc.
//
// Chosen libraries for the hub (locked in ACTION_PLAN Phase 1):
//   - HTTP router:  github.com/go-chi/chi/v5
//   - WebSocket:    github.com/gorilla/websocket   (added in Phase 1.5)
//   - Storage:      modernc.org/sqlite             (added in Phase 2, pure-Go, no CGO)
package main

import (
	"log"

	"github.com/go-chi/chi/v5"
)

func main() {
	r := chi.NewRouter()
	_ = r // routes registered in Phase 1.3+
	log.Println("lumen-hub: skeleton build — no routes registered yet")
}
