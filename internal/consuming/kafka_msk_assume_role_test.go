package consuming

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMSKAssumeRoleSessionNameRotates(t *testing.T) {
	t.Parallel()

	auth := &mskAssumeRoleAuth{}
	seen := make(map[string]struct{}, mskAssumeRoleSessionNameCount)
	for i := 0; i < mskAssumeRoleSessionNameCount*2; i++ {
		name := auth.nextSessionName()
		require.Contains(t, name, "centrifugo-msk-")
		seen[name] = struct{}{}
	}
	require.Len(t, seen, mskAssumeRoleSessionNameCount)

	require.Equal(t, "centrifugo-msk-0", auth.nextSessionName())
	require.Equal(t, "centrifugo-msk-1", auth.nextSessionName())
}
