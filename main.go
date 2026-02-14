package main

import (
	"github.com/alberto-moreno-sa/linkedin-contentful-sync/cmd"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load()
	cmd.Execute()
}
