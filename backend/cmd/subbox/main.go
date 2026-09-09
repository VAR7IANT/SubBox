package main

import (
	"fmt"
	"os"

	"github.com/VAR7IANT/SubBox/backend/internal/storage"
)

func main() {
	store, err := storage.OpenDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if err := store.Close(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
