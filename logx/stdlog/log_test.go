package stdlog

import (
	"log"
	"testing"
)

func TestDefaultLoggerUsesFullPath(t *testing.T) {
	logger := NewDefaultLog().(*Logger)
	if logger.logger.Flags()&log.Llongfile == 0 {
		t.Fatalf("flags=%d", logger.logger.Flags())
	}
}
