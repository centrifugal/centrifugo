// Package redisiam provides dynamic Redis credentials for cloud IAM authentication
// (Amazon ElastiCache, Google Cloud Memorystore and Azure Managed Redis).
package redisiam

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/centrifugal/centrifugo/v6/internal/configtypes"

	"github.com/aws/aws-sdk-go-v2/aws"
	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	awsv2cfg "github.com/aws/aws-sdk-go-v2/config"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
)

// Credentials to use for Redis AUTH.
type Credentials struct {
	Username string
	Password string
	// RefreshAfter, when non-zero, asks the Redis client to re-authenticate
	// established connections with fresh credentials at this time.
	RefreshAfter time.Time
}

// Provider returns fresh Redis credentials. It is called by the Redis client
// for every new connection (and on scheduled re-authentication).
type Provider func() (Credentials, error)

const (
	// awsTokenExpiry is the validity of ElastiCache IAM auth token, ElastiCache
	// does not accept tokens valid for longer than 15 minutes.
	awsTokenExpiry = 15 * time.Minute
	// Re-authentication interval must be less than 12 hours – ElastiCache disconnects
	// IAM authenticated connections after 12 hours unless they re-authenticate. Random
	// interval in range staggers AUTH commands of connections established together.
	awsReauthIntervalMin = 50 * time.Minute
	awsReauthIntervalMax = time.Hour
	// emptyPayloadHash is hex encoded SHA256 of an empty payload.
	emptyPayloadHash = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"

	gcpAuthScope = "https://www.googleapis.com/auth/cloud-platform"

	// credentialsTimeout bounds getting cloud credentials and tokens. Not Redis connect_timeout:
	// getting credentials for the first time (e.g. from instance metadata or an identity
	// endpoint) may take longer than establishing a Redis connection.
	credentialsTimeout = 10 * time.Second
)

// NewProvider returns Provider for the IAM auth configured in redisConf, or nil
// if no IAM auth is enabled.
func NewProvider(ctx context.Context, redisConf configtypes.Redis) (Provider, error) {
	awsConf := redisConf.AWSIAMAuth
	gcpConf := redisConf.GCPIAMAuth
	azureConf := redisConf.AzureEntraAuth
	numEnabled := 0
	for _, enabled := range []bool{awsConf.Enabled, gcpConf.Enabled, azureConf.Enabled} {
		if enabled {
			numEnabled++
		}
	}
	if numEnabled == 0 {
		return nil, nil
	}
	if numEnabled > 1 {
		return nil, errors.New("only one of aws_iam_auth, gcp_iam_auth and azure_entra_auth can be enabled")
	}
	if redisConf.Password != "" {
		return nil, errors.New("password must not be set when Redis IAM auth is enabled")
	}
	if awsConf.Enabled {
		return newAWSProvider(ctx, redisConf.User, awsConf)
	}
	if azureConf.Enabled {
		return newAzureProvider(redisConf.User, credentialsTimeout)
	}
	return newGCPProvider(redisConf.User)
}

func newAWSProvider(ctx context.Context, user string, conf configtypes.RedisAWSIAMAuth) (Provider, error) {
	if user == "" {
		return nil, errors.New("aws_iam_auth requires user to be set to the ElastiCache IAM-enabled user ID")
	}
	if (conf.ReplicationGroupID == "") == (conf.ServerlessCacheName == "") {
		return nil, errors.New("aws_iam_auth requires exactly one of replication_group_id or serverless_cache_name")
	}
	cacheName, serverless := conf.ReplicationGroupID, false
	if conf.ServerlessCacheName != "" {
		cacheName, serverless = conf.ServerlessCacheName, true
	}
	var loadOpts []func(*awsv2cfg.LoadOptions) error
	if conf.Region != "" {
		loadOpts = append(loadOpts, awsv2cfg.WithRegion(conf.Region))
	}
	awsCfg, err := awsv2cfg.LoadDefaultConfig(ctx, loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("load AWS config for Redis IAM auth: %w", err)
	}
	if awsCfg.Region == "" {
		return nil, errors.New("aws_iam_auth: AWS region not set, configure region or AWS_REGION")
	}
	// Fail fast if no AWS credentials available.
	retrieveCtx, cancel := context.WithTimeout(ctx, credentialsTimeout)
	defer cancel()
	if _, err := awsCfg.Credentials.Retrieve(retrieveCtx); err != nil {
		return nil, fmt.Errorf("aws_iam_auth: retrieve AWS credentials: %w", err)
	}
	tokenGenerator := &awsTokenGenerator{
		credentials: awsCfg.Credentials,
		signer:      v4.NewSigner(),
		region:      awsCfg.Region,
		// Cache names are lowercased by ElastiCache at creation time.
		cacheName:  strings.ToLower(cacheName),
		user:       user,
		serverless: serverless,
	}
	return func() (Credentials, error) {
		ctx, cancel := context.WithTimeout(context.Background(), credentialsTimeout)
		defer cancel()
		token, err := tokenGenerator.token(ctx, time.Now())
		if err != nil {
			return Credentials{}, fmt.Errorf("generate ElastiCache IAM auth token: %w", err)
		}
		return Credentials{
			Username:     user,
			Password:     token,
			RefreshAfter: time.Now().Add(awsReauthInterval()),
		}, nil
	}, nil
}

func awsReauthInterval() time.Duration {
	return awsReauthIntervalMin + time.Duration(randFloat64()*float64(awsReauthIntervalMax-awsReauthIntervalMin))
}

type awsTokenGenerator struct {
	credentials aws.CredentialsProvider
	signer      *v4.Signer
	region      string
	cacheName   string
	user        string
	serverless  bool
}

// token generates ElastiCache IAM auth token – SigV4 presigned connect request URL
// without the scheme. See https://docs.aws.amazon.com/AmazonElastiCache/latest/dg/auth-iam.html.
func (g *awsTokenGenerator) token(ctx context.Context, now time.Time) (string, error) {
	creds, err := g.credentials.Retrieve(ctx)
	if err != nil {
		return "", err
	}
	query := url.Values{
		"Action":        {"connect"},
		"User":          {g.user},
		"X-Amz-Expires": {fmt.Sprintf("%d", int(awsTokenExpiry.Seconds()))},
	}
	if g.serverless {
		query.Set("ResourceType", "ServerlessCache")
	}
	reqURL := url.URL{
		Scheme:   "http",
		Host:     g.cacheName,
		Path:     "/",
		RawQuery: query.Encode(),
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL.String(), nil)
	if err != nil {
		return "", err
	}
	signedURL, _, err := g.signer.PresignHTTP(ctx, creds, req, emptyPayloadHash, "elasticache", g.region, now)
	if err != nil {
		return "", err
	}
	return strings.TrimPrefix(signedURL, "http://"), nil
}

func newGCPProvider(user string) (Provider, error) {
	// Token source context is used for all future token refreshes, so it must not be canceled.
	// Token requests are bounded by the HTTP client timeout instead.
	ctx := context.WithValue(context.Background(), oauth2.HTTPClient, &http.Client{Timeout: credentialsTimeout})
	ts, err := google.DefaultTokenSource(ctx, gcpAuthScope)
	if err != nil {
		return nil, fmt.Errorf("find Google Cloud default credentials for Redis IAM auth: %w", err)
	}
	// Memorystore validates the token only when a connection authenticates, established
	// connections keep working after the token expires – so no re-authentication is scheduled.
	return newGCPTokenSourceProvider(user, ts), nil
}

func newGCPTokenSourceProvider(user string, ts oauth2.TokenSource) Provider {
	return func() (Credentials, error) {
		token, err := ts.Token()
		if err != nil {
			return Credentials{}, fmt.Errorf("get Google Cloud access token: %w", err)
		}
		return Credentials{Username: user, Password: token.AccessToken}, nil
	}
}

// randFloat64 returns a random float64 in [0, 1).
func randFloat64() float64 {
	var b [8]byte
	_, _ = rand.Read(b[:]) // Never returns an error.
	return float64(binary.LittleEndian.Uint64(b[:])>>11) / (1 << 53)
}
