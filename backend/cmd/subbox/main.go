package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/VAR7IANT/SubBox/backend/internal/api"
	"github.com/VAR7IANT/SubBox/backend/internal/auth"
	"github.com/VAR7IANT/SubBox/backend/internal/storage"
	"golang.org/x/term"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 2 && args[0] == "admin" && args[1] == "set-password" {
		return runSetPassword()
	}
	if len(args) == 1 && args[0] == "serve" {
		return runServe()
	}
	fmt.Fprintln(os.Stderr, "usage: subbox admin set-password | subbox serve")
	return 2
}

func runSetPassword() int {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "admin set-password requires an interactive terminal")
		return 2
	}
	fmt.Fprint(os.Stderr, "New credential: ")
	first, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read password: input failed")
		return 1
	}
	fmt.Fprintln(os.Stderr)
	fmt.Fprint(os.Stderr, "Confirm credential: ")
	second, err := term.ReadPassword(int(os.Stdin.Fd()))
	if err != nil {
		fmt.Fprintln(os.Stderr, "read password confirmation: input failed")
		return 1
	}
	fmt.Fprintln(os.Stderr)
	if string(first) != string(second) {
		fmt.Fprintln(os.Stderr, "passwords do not match")
		return 1
	}

	store, err := storage.OpenDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "open storage: failed")
		return 1
	}
	defer store.Close()
	service := auth.NewService(store, auth.ServiceConfig{})
	if err := service.SetAdminPassword(context.Background(), string(first)); err != nil {
		if errors.Is(err, auth.ErrPasswordPolicy) {
			fmt.Fprintln(os.Stderr, "password does not meet policy")
			return 1
		}
		fmt.Fprintln(os.Stderr, "set administrator password: failed")
		return 1
	}
	fmt.Fprintln(os.Stdout, "administrator password updated")
	return 0
}

func runServe() int {
	store, err := storage.OpenDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, "open storage: failed")
		return 1
	}
	defer store.Close()
	service := auth.NewService(store, auth.ServiceConfig{})
	handler, err := api.NewAuthHandler(service, api.LoopbackServerConfig(8080))
	if err != nil {
		fmt.Fprintln(os.Stderr, "configure server: failed")
		return 1
	}
	server := &http.Server{Addr: "127.0.0.1:8080", Handler: handler}
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)
	go func() {
		<-stop
		_ = server.Close()
	}()
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintln(os.Stderr, "serve: failed")
		return 1
	}
	return 0
}
