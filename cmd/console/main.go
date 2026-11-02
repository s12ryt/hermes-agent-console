package main

import (
	"agentconsole/internal/console"
	"agentconsole/web"
	"context"
	"errors"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func validate(password, addr string) error {
	if len(password) < 12 {
		return errors.New("CONSOLE_PASSWORD 必須至少 12 個字元")
	}
	if _, _, err := net.SplitHostPort(addr); err != nil {
		return errors.New("addr 必須為 host:port")
	}
	return nil
}
func serve() error {
	addr := flag.String("addr", "127.0.0.1:8080", "HTTP listen address")
	data := flag.String("data", "data", "Private SQLite data directory")
	secure := flag.Bool("secure-cookie", false, "Use Secure cookies behind HTTPS")
	flag.Parse()
	password := os.Getenv("CONSOLE_PASSWORD")
	if err := validate(password, *addr); err != nil {
		return err
	}
	if err := os.MkdirAll(*data, 0700); err != nil {
		return err
	}
	if err := os.Chmod(*data, 0700); err != nil {
		return err
	}
	path := filepath.Join(*data, "console.db")
	store, err := console.OpenStore(path)
	if err != nil {
		return err
	}
	defer store.Close()
	if err = os.Chmod(path, 0600); err != nil {
		return err
	}
	app := console.NewServer(store, password, *secure, web.Assets())
	defer app.Close()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	server := &http.Server{Addr: *addr, Handler: app, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 32 << 10}
	listener, err := net.Listen("tcp", *addr)
	if err != nil {
		return err
	}
	startupDone := make(chan struct{})
	go func() {
		defer close(startupDone)
		if err := app.AutoStart(ctx); err != nil {
			log.Printf("Hermes 自動連線失敗（可於設定頁重試）：%v", err)
		}
	}()
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		app.Close()
		deadline, stop := context.WithTimeout(context.Background(), 5*time.Second)
		defer stop()
		if err := server.Shutdown(deadline); err != nil {
			server.Close()
		}
	}()
	log.Printf("Hermes Console listening on %s", listener.Addr())
	err = server.Serve(listener)
	cancel()
	<-shutdownDone
	<-startupDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func main() {
	if err := serve(); err != nil {
		log.Print(err)
		os.Exit(1)
	}
}
