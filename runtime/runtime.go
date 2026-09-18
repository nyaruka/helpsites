package runtime

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	valkey "github.com/gomodule/redigo/redis"
	_ "github.com/lib/pq" // postgres driver
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/vkutil"
	"github.com/vinovest/sqlx"
)

// Runtime represents the set of services required to run helpsites. Used as a wrapper for those services to simplify
// call signatures.
type Runtime struct {
	Config *Config

	DB     *sqlx.DB
	VK     *valkey.Pool
	Dynamo *dynamodb.Client

	HTTP *HTTP
}

func NewRuntime(cfg *Config) (*Runtime, error) {
	rt := &Runtime{Config: cfg}

	var err error

	rt.DB, err = sqlx.Open("postgres", cfg.DB)
	if err != nil {
		return nil, fmt.Errorf("error creating Postgres connection pool: %w", err)
	}
	rt.DB.SetMaxIdleConns(4)
	rt.DB.SetMaxOpenConns(16)

	rt.VK, err = vkutil.NewPool(cfg.Valkey, vkutil.WithMaxActive(16))
	if err != nil {
		return nil, fmt.Errorf("error creating Valkey pool: %w", err)
	}

	// the AWS service constructors resolve credentials and region from the SDK default chain (env
	// vars, instance/task IAM role, shared config/credentials files, etc.)
	rt.Dynamo, err = dynamo.NewClient(context.Background(), cfg.DynamoEndpoint)
	if err != nil {
		return nil, fmt.Errorf("error creating DynamoDB client: %w", err)
	}

	rt.HTTP = newHTTP(cfg)

	return rt, nil
}

func (r *Runtime) Stop() {
	r.DB.Close()
	r.VK.Close()
}
