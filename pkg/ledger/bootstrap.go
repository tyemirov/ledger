package ledger

import (
	"fmt"
	"sort"
)

const bootstrapIdempotencySuffix = "bootstrap_grant"

type BootstrapGrantRule struct {
	tenantID           TenantID
	ledgerID           LedgerID
	amount             PositiveAmountCents
	idempotencyKeyBase IdempotencyKey
	metadata           MetadataJSON
}

func NewBootstrapGrantRule(tenantID TenantID, ledgerID LedgerID, amount PositiveAmountCents, idempotencyKeyBase IdempotencyKey, metadata MetadataJSON) (BootstrapGrantRule, error) {
	if err := validateIdentifierValue(tenantID.value, ErrInvalidTenantID); err != nil {
		return BootstrapGrantRule{}, err
	}
	if err := validateIdentifierValue(ledgerID.value, ErrInvalidLedgerID); err != nil {
		return BootstrapGrantRule{}, err
	}
	if err := validatePositiveAmount(amount); err != nil {
		return BootstrapGrantRule{}, err
	}
	if err := validateIdentifierValue(idempotencyKeyBase.value, ErrInvalidIdempotencyKey); err != nil {
		return BootstrapGrantRule{}, err
	}
	if err := validateIdentifierValue(metadata.value, ErrInvalidMetadataJSON); err != nil {
		return BootstrapGrantRule{}, err
	}
	return BootstrapGrantRule{
		tenantID:           tenantID,
		ledgerID:           ledgerID,
		amount:             amount,
		idempotencyKeyBase: idempotencyKeyBase,
		metadata:           metadata,
	}, nil
}

func (rule BootstrapGrantRule) TenantID() TenantID {
	return rule.tenantID
}

func (rule BootstrapGrantRule) LedgerID() LedgerID {
	return rule.ledgerID
}

func (rule BootstrapGrantRule) Amount() PositiveAmountCents {
	return rule.amount
}

func (rule BootstrapGrantRule) IdempotencyKeyBase() IdempotencyKey {
	return rule.idempotencyKeyBase
}

func (rule BootstrapGrantRule) Metadata() MetadataJSON {
	return rule.metadata
}

func (rule BootstrapGrantRule) BootstrapIdempotencyKey() (IdempotencyKey, error) {
	return deriveIdempotencyKey(rule.IdempotencyKeyBase(), bootstrapIdempotencySuffix)
}

type bootstrapGrantScope struct {
	tenantID TenantID
	ledgerID LedgerID
}

type BootstrapGrantPolicy struct {
	rules map[bootstrapGrantScope]BootstrapGrantRule
}

func NewBootstrapGrantPolicy(rules []BootstrapGrantRule) (BootstrapGrantPolicy, error) {
	if len(rules) == 0 {
		return BootstrapGrantPolicy{}, nil
	}
	policy := BootstrapGrantPolicy{
		rules: make(map[bootstrapGrantScope]BootstrapGrantRule, len(rules)),
	}
	for _, rule := range rules {
		scope := bootstrapGrantScope{
			tenantID: rule.TenantID(),
			ledgerID: rule.LedgerID(),
		}
		if _, exists := policy.rules[scope]; exists {
			return BootstrapGrantPolicy{}, fmt.Errorf("%w: duplicate bootstrap rule for tenant_id=%s ledger_id=%s", ErrInvalidServiceConfig, rule.TenantID().String(), rule.LedgerID().String())
		}
		policy.rules[scope] = rule
	}
	return policy, nil
}

func (policy BootstrapGrantPolicy) ruleFor(tenantID TenantID, ledgerID LedgerID) (BootstrapGrantRule, bool) {
	if policy.rules == nil {
		return BootstrapGrantRule{}, false
	}
	rule, ok := policy.rules[bootstrapGrantScope{tenantID: tenantID, ledgerID: ledgerID}]
	return rule, ok
}

func (policy BootstrapGrantPolicy) Rules() []BootstrapGrantRule {
	if policy.rules == nil {
		return nil
	}
	rules := make([]BootstrapGrantRule, 0, len(policy.rules))
	for _, rule := range policy.rules {
		rules = append(rules, rule)
	}
	sort.Slice(rules, func(i, j int) bool {
		if rules[i].TenantID().String() == rules[j].TenantID().String() {
			return rules[i].LedgerID().String() < rules[j].LedgerID().String()
		}
		return rules[i].TenantID().String() < rules[j].TenantID().String()
	})
	return rules
}
