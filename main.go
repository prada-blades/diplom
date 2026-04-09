package main

import (
	"log"

	httpapi "diplom/internal/http"
)

func main() {
	app, err := httpapi.NewApp()
	if err != nil {
		log.Fatalf("init app: %v", err)
	}

	if err := app.Run(); err != nil {
		log.Fatalf("run app: %v", err)
	}
}
