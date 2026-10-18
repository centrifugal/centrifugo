package confighelpers

import (
	"strings"
	"testing"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/centrifugal/centrifuge"
	"github.com/stretchr/testify/require"
)

func TestGetRedisShardConfigsAWSIAMAuth(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	shardConfigs, err := getRedisShardConfigs(configtypes.Redis{
		ClusterAddress: []string{"127.0.0.1:7000,127.0.0.1:7001"},
		User:           "iam-user-01",
		AWSIAMAuth:     configtypes.RedisAWSIAMAuth{Enabled: true, ServerlessCacheName: "cache-01", Region: "us-east-1"},
	})
	require.NoError(t, err)
	require.Len(t, shardConfigs, 1)
	require.NotNil(t, shardConfigs[0].AuthCredentialsFn)
	creds, err := shardConfigs[0].AuthCredentialsFn(centrifuge.RedisAuthCredentialsContext{})
	require.NoError(t, err)
	require.Equal(t, "iam-user-01", creds.Username)
	require.True(t, strings.HasPrefix(creds.Password, "cache-01/?Action=connect&ResourceType=ServerlessCache&User=iam-user-01&"))
	require.False(t, creds.RefreshAfter.IsZero())
}

func TestGetRedisShardConfigsBadIAMAuth(t *testing.T) {
	_, err := getRedisShardConfigs(configtypes.Redis{
		Password:   "pass",
		GCPIAMAuth: configtypes.RedisGCPIAMAuth{Enabled: true},
	})
	require.ErrorContains(t, err, "password must not be set")
}

func TestMergeModes(t *testing.T) {
	tests := []struct {
		input    []string
		expected string
	}{
		{
			input:    []string{"cluster", "cluster", "standalone", "sentinel", "sentinel", "cluster"},
			expected: "cluster-x2,standalone,sentinel-x2,cluster",
		},
		{
			input:    []string{"standalone"},
			expected: "standalone",
		},
		{
			input:    []string{"sentinel", "sentinel", "sentinel"},
			expected: "sentinel-x3",
		},
		{
			input:    []string{"cluster", "standalone", "sentinel"},
			expected: "cluster,standalone,sentinel",
		},
		{
			input:    []string{},
			expected: "",
		},
		{
			input:    []string{"a", "a", "a", "b", "b", "c", "a"},
			expected: "a-x3,b-x2,c,a",
		},
		{
			input:    []string{"a", "b", "b", "b", "c", "c", "d", "d", "d", "d"},
			expected: "a,b-x3,c-x2,d-x4",
		},
	}

	for _, test := range tests {
		got := mergeModes(test.input)
		if got != test.expected {
			t.Errorf("mergeModes(%v) = %q; want %q", test.input, got, test.expected)
		}
	}
}
