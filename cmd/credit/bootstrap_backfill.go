package main

import (
	"context"
	"errors"
	"fmt"
	"os/signal"
	"syscall"
	"time"

	"github.com/MarkoPoloResearchLab/ledger/internal/store/gormstore"
	"github.com/MarkoPoloResearchLab/ledger/pkg/ledger"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

const defaultBootstrapBackfillPageSize = 1000

type bootstrapBackfillSummary struct {
	Scopes       int
	AccountsSeen int
	Applied      int
	Skipped      int
}

func newBootstrapBackfillCommand(cfg *runtimeConfig) *cobra.Command {
	var pageSize int
	cmd := &cobra.Command{
		Use:           "bootstrap-backfill",
		Short:         "Apply configured bootstrap grants to existing accounts missing them",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			logger, err := zap.NewProduction()
			if err != nil {
				return fmt.Errorf("logger init: %w", err)
			}
			defer func() { _ = logger.Sync() }()

			ctx, stop := signal.NotifyContext(cmd.Context(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()

			summary, err := runBootstrapBackfill(ctx, cfg, logger, pageSize)
			if err != nil {
				return err
			}
			logger.Info("bootstrap backfill complete",
				zap.Int("scopes", summary.Scopes),
				zap.Int("accounts_seen", summary.AccountsSeen),
				zap.Int("applied", summary.Applied),
				zap.Int("skipped", summary.Skipped),
			)
			return nil
		},
	}

	cmd.Flags().IntVar(&pageSize, "page-size", defaultBootstrapBackfillPageSize, "Accounts page size for backfill")
	return cmd
}

func runBootstrapBackfill(ctx context.Context, cfg *runtimeConfig, logger *zap.Logger, pageSize int) (bootstrapBackfillSummary, error) {
	if pageSize <= 0 {
		return bootstrapBackfillSummary{}, fmt.Errorf("page size must be positive")
	}
	bootstrapPolicy, err := parseBootstrapGrantPolicy(cfg.BootstrapGrantsJSON)
	if err != nil {
		return bootstrapBackfillSummary{}, err
	}
	rules := bootstrapPolicy.Rules()
	if len(rules) == 0 {
		return bootstrapBackfillSummary{}, fmt.Errorf("%w: bootstrap grants json is empty", ledger.ErrInvalidServiceConfig)
	}

	gormDB, cleanup, driver, err := openDatabase(ctx, cfg.DatabaseURL)
	if err != nil {
		return bootstrapBackfillSummary{}, fmt.Errorf("database open: %w", err)
	}
	defer cleanup()

	if err := prepareSchema(gormDB, driver); err != nil {
		return bootstrapBackfillSummary{}, err
	}

	store := gormstore.New(gormDB)
	nowFn := func() int64 { return time.Now().UTC().Unix() }

	summary := bootstrapBackfillSummary{Scopes: len(rules)}
	for _, rule := range rules {
		scopeApplied, scopeSkipped, scopeSeen, err := backfillBootstrapRule(ctx, store, rule, nowFn, pageSize)
		if err != nil {
			return bootstrapBackfillSummary{}, err
		}
		summary.AccountsSeen += scopeSeen
		summary.Applied += scopeApplied
		summary.Skipped += scopeSkipped

		logger.Info("bootstrap backfill scope complete",
			zap.String("tenant_id", rule.TenantID().String()),
			zap.String("ledger_id", rule.LedgerID().String()),
			zap.Int("accounts_seen", scopeSeen),
			zap.Int("applied", scopeApplied),
			zap.Int("skipped", scopeSkipped),
		)
	}
	return summary, nil
}

func backfillBootstrapRule(ctx context.Context, store *gormstore.Store, rule ledger.BootstrapGrantRule, nowFn func() int64, pageSize int) (applied int, skipped int, seen int, err error) {
	var cursor *ledger.AccountID
	for {
		accounts, err := store.ListAccountSummaries(ctx, rule.TenantID(), rule.LedgerID(), cursor, pageSize)
		if err != nil {
			return 0, 0, 0, err
		}
		if len(accounts) == 0 {
			return applied, skipped, seen, nil
		}

		for _, account := range accounts {
			seen++
			changed, err := applyBootstrapGrantToAccount(ctx, store, account.AccountID(), rule, nowFn)
			if err != nil {
				return 0, 0, 0, err
			}
			if changed {
				applied++
			} else {
				skipped++
			}
		}

		lastAccountID := accounts[len(accounts)-1].AccountID()
		cursor = &lastAccountID
	}
}

func applyBootstrapGrantToAccount(ctx context.Context, store ledger.Store, accountID ledger.AccountID, rule ledger.BootstrapGrantRule, nowFn func() int64) (bool, error) {
	bootstrapIdempotencyKey, err := rule.BootstrapIdempotencyKey()
	if err != nil {
		return false, err
	}

	applied := false
	operationErr := store.WithTx(ctx, func(ctx context.Context, txStore ledger.Store) error {
		existing, lookupErr := txStore.GetEntryByIdempotencyKey(ctx, accountID, bootstrapIdempotencyKey)
		if lookupErr == nil {
			if existing.Type() != ledger.EntryGrant {
				return ledger.ErrDuplicateIdempotencyKey
			}
			return nil
		}
		if !errors.Is(lookupErr, ledger.ErrUnknownEntry) {
			return lookupErr
		}

		entryInput, err := ledger.NewEntryInput(
			accountID,
			ledger.EntryGrant,
			rule.Amount().ToEntryAmountCents(),
			nil,
			nil,
			bootstrapIdempotencyKey,
			0,
			rule.Metadata(),
			nowFn(),
		)
		if err != nil {
			return err
		}
		_, insertErr := txStore.InsertEntry(ctx, entryInput)
		if insertErr == nil {
			applied = true
			return nil
		}
		if !errors.Is(insertErr, ledger.ErrDuplicateIdempotencyKey) {
			return insertErr
		}
		existing, lookupErr = txStore.GetEntryByIdempotencyKey(ctx, accountID, bootstrapIdempotencyKey)
		if lookupErr != nil {
			return errors.Join(insertErr, lookupErr)
		}
		if existing.Type() != ledger.EntryGrant {
			return ledger.ErrDuplicateIdempotencyKey
		}
		return nil
	})
	if operationErr != nil {
		return false, operationErr
	}
	return applied, nil
}
