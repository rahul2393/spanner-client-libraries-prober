package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
)

func startPprofServer(cfg config) {
	if !cfg.enablePprof {
		return
	}
	go func() {
		log.Printf("pprof listening on %s", cfg.pprofAddr)
		if err := http.ListenAndServe(cfg.pprofAddr, nil); err != nil {
			log.Printf("pprof server stopped: %v", err)
		}
	}()
}
