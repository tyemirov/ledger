package main

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/pgstore"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
)

const (
	environmentKeyDatabaseURL = "DATABASE_URL"
	environmentKeyListenAddr  = "GRPC_LISTEN_ADDR"
	defaultDatabaseURL        = "postgres://postgres:postgres@localhost:5432/credit?sslmode=disable"
	defaultGRPCListenAddress  = ":7000"
)

// main starts the gRPC credit service.
func main() {
	databaseURL := envOrDefault(environmentKeyDatabaseURL, defaultDatabaseURL)
	listenAddress := envOrDefault(environmentKeyListenAddr, defaultGRPCListenAddress)

	requestContext := context.Background()
	connectionPool, poolError := pgxpool.New(requestContext, databaseURL)
	if poolError != nil {
		log.Fatalf("database pool init: %v", poolError)
	}
	defer connectionPool.Close()

	store := pgstore.New(connectionPool)
	nowUnixSeconds := func() int64 { return time.Now().UTC().Unix() }
	creditService := credit.NewService(store, nowUnixSeconds)

	listener, listenError := net.Listen("tcp", listenAddress)
	if listenError != nil {
		log.Fatalf("listen: %v", listenError)
	}
	grpcServer := grpc.NewServer()
	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(creditService))

	log.Printf("listening on %s", listenAddress)
	if serveError := grpcServer.Serve(listener); serveError != nil {
		log.Fatalf("serve: %v", serveError)
	}
}

func envOrDefault(key string, fallback string) string {
	value, ok := os.LookupEnv(key)
	if !ok || value == "" {
		return fallback
	}
	return value
}

func mustAtoi(value string, fallback int) int {
	if value == "" {
		return fallback
	}
	parsed, parseError := strconv.Atoi(value)
	if parseError != nil {
		return fallback
	}
	return parsed
}
