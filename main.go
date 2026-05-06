package main

import (
	"crypto/tls"
	"flag"
	"log/slog"
	"net"
	"os"
)

func main() {
	cfgPath := flag.String("c", "", "config file path (YAML), defaults to built-in defaults")
	dataDir := flag.String("d", "", "data directory for TLS cert (default: ~/.163-wrapper)")
	flag.Parse()

	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	level := slog.LevelInfo
	if cfg.LogLevel == "debug" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	tlsCfg, certPath, err := LoadOrCreateTLSConfig(*dataDir)
	if err != nil {
		slog.Error("TLS setup failed", "err", err)
		os.Exit(1)
	}
	slog.Info("TLS cert ready", "path", certPath)
	slog.Info("trust cert on macOS (run once):",
		"cmd", "sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain "+certPath)

	rawLn, err := net.Listen("tcp", cfg.Listen)
	if err != nil {
		slog.Error("listen failed", "addr", cfg.Listen, "err", err)
		os.Exit(1)
	}
	ln := tls.NewListener(rawLn, tlsCfg)
	slog.Info("163 IMAP proxy listening (TLS)", "addr", cfg.Listen, "upstream", cfg.Upstream)

	for {
		conn, err := ln.Accept()
		if err != nil {
			slog.Error("accept error", "err", err)
			continue
		}
		go handleConn(conn, cfg)
	}
}
