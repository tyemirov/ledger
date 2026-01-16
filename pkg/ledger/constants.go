package ledger

const (
	operationGrant   = "grant"
	operationReserve = "reserve"
	operationCapture = "capture"
	operationRelease = "release"
	operationSpend   = "spend"

	operationStatusOK    = "ok"
	operationStatusError = "error"

	idempotencyKeyDelimiter  = ":"
	idempotencySuffixReverse = "reverse"
	idempotencySuffixSpend   = "spend"
)
