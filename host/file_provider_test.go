package host

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestFileProvider(t *testing.T) {
	t.Parallel()
	logger, err := zap.NewDevelopment()
	require.NoError(t, err)
	defer func() {
		_ = logger.Sync()
	}()

	p := NewFileProvider("./testdata/hosts.txt", logger)

	cnt := 0
	register := func(_ Host) {
		cnt++
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err = p.Run(ctx, register, nil)
	require.NoError(t, err)
	require.Eventually(t, func() bool { return cnt > 1 }, 1*time.Second, 50*time.Millisecond)
}
