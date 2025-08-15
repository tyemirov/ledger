// cmd/credit/main.go
package main

import (
	"log"
	"net"
	"os"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/api/credit/v1"
	"github.com/MarkoPoloResearchLab/ledger/internal/credit"
	"github.com/MarkoPoloResearchLab/ledger/internal/grpcserver"
	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"google.golang.org/grpc"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

const (
	environmentKeyDatabaseURL = "DATABASE_URL"
	environmentKeyListenAddr  = "GRPC_LISTEN_ADDR"
	defaultDatabaseURL        = "postgres://postgres:postgres@localhost:5432/credit?sslmode=disable"
	defaultGRPCListenAddress  = ":7000"
)

func main() {
	dsn := envOrDefault(environmentKeyDatabaseURL, defaultDatabaseURL)
	listenAddress := envOrDefault(environmentKeyListenAddr, defaultGRPCListenAddress)

	// Open GORM
	gdb, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("gorm open: %v", err)
	}
	// Optional: ensure underlying pool closes on exit
	sqlDB, err := gdb.DB()
	if err != nil {
		log.Fatalf("gorm db(): %v", err)
	}
	defer sqlDB.Close()

	// AutoMigrate for tables only (we keep your enum & indexes from migrations.sql)
	if err := gdb.AutoMigrate(&gormstore.Account{}, &gormstore.LedgerEntry{}); err != nil {
		log.Fatalf("automigrate: %v", err)
	}

	store := gormstore.New(gdb)
	nowUnixSeconds := func() int64 { return time.Now().UTC().Unix() }
	creditService := credit.NewService(store, nowUnixSeconds)

	l, err := net.Listen("tcp", listenAddress)
	if err != nil {
		log.Fatalf("listen: %v", err)
	}
	grpcServer := grpc.NewServer()
	creditv1.RegisterCreditServiceServer(grpcServer, grpcserver.NewCreditServiceServer(creditService))

	log.Printf("listening on %s", listenAddress)
	if serveError := grpcServer.Serve(l); serveError != nil {
		log.Fatalf("serve: %v", serveError)
	}
}

func envOrDefault(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}
