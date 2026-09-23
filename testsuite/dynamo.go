package testsuite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/stretchr/testify/require"
)

// ensureDynamoTable creates this binary's certificates table if it doesn't exist, and empties it. Outside of tests the
// table is created by temba's migrate_dynamo command.
func ensureDynamoTable(t *testing.T, rt *runtime.Runtime) {
	t.Helper()

	ctx := context.Background()
	table := aws.String(rt.Config.CertsTable())

	_, err := rt.Dynamo.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: table})
	if notFound := (*types.ResourceNotFoundException)(nil); errors.As(err, &notFound) {
		_, err = rt.Dynamo.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName:            table,
			AttributeDefinitions: []types.AttributeDefinition{{AttributeName: aws.String("Key"), AttributeType: types.ScalarAttributeTypeS}},
			KeySchema:            []types.KeySchemaElement{{AttributeName: aws.String("Key"), KeyType: types.KeyTypeHash}},
			BillingMode:          types.BillingModePayPerRequest,
		})
		require.NoError(t, err, "error creating DynamoDB table")

		err = dynamodb.NewTableExistsWaiter(rt.Dynamo).Wait(ctx, &dynamodb.DescribeTableInput{TableName: table}, time.Minute)
		require.NoError(t, err, "error waiting for DynamoDB table")
		return
	}
	require.NoError(t, err, "error describing DynamoDB table")

	paginator := dynamodb.NewScanPaginator(rt.Dynamo, &dynamodb.ScanInput{
		TableName: table, ProjectionExpression: aws.String("#k"), ExpressionAttributeNames: map[string]string{"#k": "Key"},
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		require.NoError(t, err)
		for _, item := range page.Items {
			_, err := rt.Dynamo.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: table, Key: map[string]types.AttributeValue{"Key": item["Key"]}})
			require.NoError(t, err)
		}
	}
}
