package main

import (
	"github.com/htserhsluk/emergency-response-matching/internal/app"
	"log"
)

func main() {
	if err := app.Run("api"); err != nil {
		log.Fatal(err)
	}
}
