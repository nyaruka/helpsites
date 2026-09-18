package testsuite

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/gocommon/aws/dynamo"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/stretchr/testify/require"
)

// EnsureDynamoTable creates the certificates table in the test DynamoDB if it doesn't exist, empties it, and returns a
// client for it. The test runtime is self-signed so has no client of its own.
func EnsureDynamoTable(t *testing.T, rt *runtime.Runtime) *dynamodb.Client {
	t.Helper()

	ctx := context.Background()
	table := aws.String(rt.Config.CertsTable())

	client, err := dynamo.NewClient(ctx, rt.Config.DynamoEndpoint)
	require.NoError(t, err)

	_, err = client.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: table})
	var notFound *types.ResourceNotFoundException
	if errors.As(err, &notFound) {
		_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName:            table,
			AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("Key"), AttributeType: types.ScalarAttributeTypeS}},
			KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("Key"), KeyType: types.KeyTypeHash}},
			BillingMode:          types.BillingModePayPerRequest,
		})
	}
	require.NoError(t, err, "error ensuring DynamoDB table")

	// empty it
	paginator := dynamodb.NewScanPaginator(client, &dynamodb.ScanInput{TableName: table, ProjectionExpression: aws.String("#k"), ExpressionAttributeNames: map[string]string{"#k": "Key"}})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		require.NoError(t, err)
		for _, item := range page.Items {
			_, err := client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: table, Key: map[string]types.AttributeValue{"Key": item["Key"]}})
			require.NoError(t, err)
		}
	}

	return client
}
