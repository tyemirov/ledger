package ledger

import "context"

// ServiceOption configures a Service instance.
type ServiceOption func(*Service)

// OperationLogger records domain-level events emitted by Service operations.
type OperationLogger interface {
	LogOperation(ctx context.Context, entry OperationLog)
}

// OperationLog describes a state-changing ledger operation.
type OperationLog struct {
	Operation      string
	UserID         UserID
	ReservationID  ReservationID
	Amount         AmountCents
	IdempotencyKey IdempotencyKey
	Metadata       MetadataJSON
	Status         string
	Error          error
}

// WithOperationLogger wires a logger that receives callbacks for every operation.
func WithOperationLogger(logger OperationLogger) ServiceOption {
	return func(service *Service) {
		service.logger = logger
	}
}
