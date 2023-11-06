package main

import (
	_ "embed"
	"log"
	"time"

	"github.com/alexanderi96/gptron/session"
)

func main() {
	log.Printf("Running GPTronBot...")

	for {
		log.Println(session.Dsp.Poll())
		log.Printf("Lost connection, waiting one minute...")

		time.Sleep(1 * time.Minute)
	}
}
