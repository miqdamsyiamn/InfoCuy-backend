package main

import (
	"fmt"
	"os"

	// Import package dari folder api
	// SESUAIKAN "InfoCuy-Backend" DENGAN NAMA MODULE DI go.mod KAMU
	"InfoCuy-Backend/api" 

	"github.com/joho/godotenv"
)

func main() {
	// Load .env di local
	err := godotenv.Load()
	if err != nil {
		fmt.Println("Info: .env not found")
	}

	// Panggil Router dari package api (handler)
	r := handler.SetupRouter()

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Println("🚀 Server running on port " + port)
	r.Run(":" + port)
}