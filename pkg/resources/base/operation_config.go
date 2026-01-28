package base

// OperationConfig defines operation semantics
type OperationConfig struct {
	Synchronous            bool
	OperationIDExtractor   func(response map[string]interface{}) string
	OperationURLBuilder    func(ctx PathContext, operationID string) string
	NativeIDExtractor      func(response map[string]interface{}, ctx PathContext) string
	OperationStatusChecker func(response map[string]interface{}) (done bool, err error)
	PostMutationHook       func(ctx PathContext) error
}
