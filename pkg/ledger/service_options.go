package ledger

// WithBootstrapGrantPolicy configures server-managed bootstrap grants for accounts.
func WithBootstrapGrantPolicy(policy BootstrapGrantPolicy) ServiceOption {
	return func(service *Service) {
		service.bootstrapPolicy = policy
	}
}
