package redisiam

import (
	"context"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/stretchr/testify/require"
	"golang.org/x/oauth2"
)

func TestNewProviderValidation(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	testCases := []struct {
		name    string
		conf    configtypes.Redis
		wantErr string
		wantNil bool
	}{
		{
			name:    "disabled",
			conf:    configtypes.Redis{User: "user", Password: "pass"},
			wantNil: true,
		},
		{
			name: "both enabled",
			conf: configtypes.Redis{
				AWSIAMAuth: configtypes.RedisAWSIAMAuth{Enabled: true, ReplicationGroupID: "cache", Region: "us-east-1"},
				GCPIAMAuth: configtypes.RedisGCPIAMAuth{Enabled: true},
			},
			wantErr: "only one of aws_iam_auth, gcp_iam_auth and azure_entra_auth can be enabled",
		},
		{
			name: "azure and gcp enabled",
			conf: configtypes.Redis{
				GCPIAMAuth:     configtypes.RedisGCPIAMAuth{Enabled: true},
				AzureEntraAuth: configtypes.RedisAzureEntraAuth{Enabled: true},
			},
			wantErr: "only one of aws_iam_auth, gcp_iam_auth and azure_entra_auth can be enabled",
		},
		{
			name: "azure password set",
			conf: configtypes.Redis{
				Password:       "pass",
				AzureEntraAuth: configtypes.RedisAzureEntraAuth{Enabled: true},
			},
			wantErr: "password must not be set",
		},
		{
			name: "password set",
			conf: configtypes.Redis{
				User:       "user",
				Password:   "pass",
				AWSIAMAuth: configtypes.RedisAWSIAMAuth{Enabled: true, ReplicationGroupID: "cache", Region: "us-east-1"},
			},
			wantErr: "password must not be set",
		},
		{
			name: "aws no user",
			conf: configtypes.Redis{
				AWSIAMAuth: configtypes.RedisAWSIAMAuth{Enabled: true, ReplicationGroupID: "cache", Region: "us-east-1"},
			},
			wantErr: "requires user",
		},
		{
			name: "aws no cache",
			conf: configtypes.Redis{
				User:       "user",
				AWSIAMAuth: configtypes.RedisAWSIAMAuth{Enabled: true, Region: "us-east-1"},
			},
			wantErr: "requires exactly one of replication_group_id or serverless_cache_name",
		},
		{
			name: "aws both caches",
			conf: configtypes.Redis{
				User: "user",
				AWSIAMAuth: configtypes.RedisAWSIAMAuth{
					Enabled: true, ReplicationGroupID: "group", ServerlessCacheName: "cache", Region: "us-east-1",
				},
			},
			wantErr: "requires exactly one of replication_group_id or serverless_cache_name",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			provider, err := NewProvider(context.Background(), tc.conf)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.wantNil {
				require.Nil(t, provider)
			}
		})
	}
}

func TestAWSProvider(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "AKIDEXAMPLE")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "secret")

	provider, err := NewProvider(context.Background(), configtypes.Redis{
		User: "iam-user-01",
		AWSIAMAuth: configtypes.RedisAWSIAMAuth{
			Enabled:            true,
			ReplicationGroupID: "My-Cache",
			Region:             "eu-west-1",
		},
	})
	require.NoError(t, err)
	require.NotNil(t, provider)

	before := time.Now()
	creds, err := provider()
	require.NoError(t, err)
	require.Equal(t, "iam-user-01", creds.Username)
	require.True(t, strings.HasPrefix(creds.Password, "my-cache/?"), creds.Password)
	require.False(t, creds.RefreshAfter.Before(before.Add(awsReauthIntervalMin)))
	require.True(t, creds.RefreshAfter.Before(before.Add(12*time.Hour)))
}

func TestAWSProviderNoCredentials(t *testing.T) {
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", t.TempDir()+"/credentials")
	t.Setenv("AWS_CONFIG_FILE", t.TempDir()+"/config")
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_FULL_URI", "")
	t.Setenv("AWS_CONTAINER_CREDENTIALS_RELATIVE_URI", "")
	t.Setenv("AWS_WEB_IDENTITY_TOKEN_FILE", "")

	_, err := NewProvider(context.Background(), configtypes.Redis{
		User:       "iam-user-01",
		AWSIAMAuth: configtypes.RedisAWSIAMAuth{Enabled: true, ReplicationGroupID: "group", Region: "us-east-1"},
	})
	require.ErrorContains(t, err, "retrieve AWS credentials")
}

func TestAWSReauthInterval(t *testing.T) {
	for i := 0; i < 100; i++ {
		interval := awsReauthInterval()
		require.GreaterOrEqual(t, interval, awsReauthIntervalMin)
		require.Less(t, interval, awsReauthIntervalMax)
	}
}

func TestAWSTokenGenerator(t *testing.T) {
	now := time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC)

	for _, serverless := range []bool{false, true} {
		g := &awsTokenGenerator{
			credentials: aws.NewCredentialsCache(credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", "session")),
			signer:      v4.NewSigner(),
			region:      "us-east-1",
			cacheName:   "cache-01",
			user:        "iam-user-01",
			serverless:  serverless,
		}
		token, err := g.token(context.Background(), now)
		require.NoError(t, err)
		require.False(t, strings.HasPrefix(token, "http://"))

		u, err := url.Parse("http://" + token)
		require.NoError(t, err)
		require.Equal(t, "cache-01", u.Host)
		require.Equal(t, "/", u.Path)
		q := u.Query()
		require.Equal(t, "connect", q.Get("Action"))
		require.Equal(t, "iam-user-01", q.Get("User"))
		require.Equal(t, "900", q.Get("X-Amz-Expires"))
		require.Equal(t, "AWS4-HMAC-SHA256", q.Get("X-Amz-Algorithm"))
		require.Equal(t, "AKIDEXAMPLE/20261018/us-east-1/elasticache/aws4_request", q.Get("X-Amz-Credential"))
		require.Equal(t, "20261018T120000Z", q.Get("X-Amz-Date"))
		require.Equal(t, "session", q.Get("X-Amz-Security-Token"))
		require.NotEmpty(t, q.Get("X-Amz-Signature"))
		if serverless {
			require.Equal(t, "ServerlessCache", q.Get("ResourceType"))
		} else {
			require.False(t, q.Has("ResourceType"))
		}
	}
}

// Expected token generated by botocore RequestSigner.generate_presigned_url for the same input.
func TestAWSTokenGeneratorReference(t *testing.T) {
	g := &awsTokenGenerator{
		credentials: credentials.NewStaticCredentialsProvider("AKIDEXAMPLE", "secret", ""),
		signer:      v4.NewSigner(),
		region:      "us-east-1",
		cacheName:   "cache-01",
		user:        "iam-user-01",
	}
	token, err := g.token(context.Background(), time.Date(2026, 10, 18, 12, 0, 0, 0, time.UTC))
	require.NoError(t, err)
	require.Equal(t, "cache-01/?Action=connect&User=iam-user-01&X-Amz-Algorithm=AWS4-HMAC-SHA256"+
		"&X-Amz-Credential=AKIDEXAMPLE%2F20261018%2Fus-east-1%2Felasticache%2Faws4_request"+
		"&X-Amz-Date=20261018T120000Z&X-Amz-Expires=900&X-Amz-SignedHeaders=host"+
		"&X-Amz-Signature=d1f892ee00cc91d75953a750345096c372638bc18baff00631bf220d5a50a026", token)
}

func TestGCPTokenSourceProvider(t *testing.T) {
	provider := newGCPTokenSourceProvider("", oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "access-token"}))
	creds, err := provider()
	require.NoError(t, err)
	require.Empty(t, creds.Username)
	require.Equal(t, "access-token", creds.Password)
	require.True(t, creds.RefreshAfter.IsZero())
}
