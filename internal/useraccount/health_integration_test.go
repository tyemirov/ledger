package useraccount_test

import (
	"testing"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/internal/useraccount"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestAccountDatastoreHealth(test *testing.T) {
	database, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		test.Fatal(err)
	}
	service, err := useraccount.NewDefaultService(gormstore.New(database))
	if err != nil {
		test.Fatal(err)
	}
	if err := service.CheckHealth(test.Context()); err != nil {
		test.Fatal(err)
	}
	pool, err := database.DB()
	if err != nil {
		test.Fatal(err)
	}
	if err := pool.Close(); err != nil {
		test.Fatal(err)
	}
	if err := service.CheckHealth(test.Context()); err == nil {
		test.Fatal("closed datastore reported healthy")
	}
}
