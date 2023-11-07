package main

import (
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/alexanderi96/gptron/session"
)

func init() {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt)
	go func() {
		for sig := range c {
			if sig == syscall.SIGINT {
				log.Printf("Captured %v. Exiting...", sig)
				os.Exit(0)
			}
		}
	}()
}

func main() {
	log.Printf("Running GPTronBot...")

	for {
		log.Println(session.Dsp.Poll())
		log.Printf("Lost connection, waiting one minute...")

		time.Sleep(1 * time.Minute)
	}
}
