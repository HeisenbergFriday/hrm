package main

import (
	"log"

	"peopleops/internal/config"
	"peopleops/internal/database"
)

func main() {
	if err := config.Load(); err != nil {
		log.Printf("load env warning: %v", err)
	}
	if err := database.Init(); err != nil {
		log.Fatalf("apply database migrations failed: %v", err)
	}
	log.Println("database migrations and seed presets applied")
}
