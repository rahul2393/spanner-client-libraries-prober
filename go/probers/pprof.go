package main

import (
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
)

func startPprofServer(cfg config) {
	if !cfg.enablePprof {
		return
	}
	mutexFraction := getEnvInt("PPROF_MUTEX_PROFILE_FRACTION", 5)
	if mutexFraction > 0 {
		runtime.SetMutexProfileFraction(mutexFraction)
	}
	blockRate := getEnvInt("PPROF_BLOCK_PROFILE_RATE", 1000000)
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
	}
	log.Printf("pprof profiling enabled addr=%s mutex_fraction=%d block_rate=%d", cfg.pprofAddr, mutexFraction, blockRate)
	go func() {
		log.Printf("pprof listening on %s", cfg.pprofAddr)
		if err := http.ListenAndServe(cfg.pprofAddr, nil); err != nil {
			log.Printf("pprof server stopped: %v", err)
		}
	}()
}
