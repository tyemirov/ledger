package ledger

const (
	operationGrant   = "grant"
	operationReserve = "reserve"
	operationCapture = "capture"
	operationRelease = "release"
	operationSpend   = "spend"
	operationRefund  = "refund"

	operationStatusOK    = "ok"
	operationStatusError = "error"

	idempotencyKeyDelimiter  = ":"
	idempotencySuffixReverse = "reverse"
	idempotencySuffixSpend   = "spend"
)
