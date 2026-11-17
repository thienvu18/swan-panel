package vici

// SetReadProcStatusForTest allows unit tests to mock Linux /proc/<pid>/status reading
func SetReadProcStatusForTest(fn func(name string) ([]byte, error)) func() {
	prev := readProcStatus
	readProcStatus = fn
	return func() {
		readProcStatus = prev
	}
}
