package testsuite

import (
	"context"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/nyaruka/helpsites/core/certs"
	"github.com/nyaruka/helpsites/runtime"
	"github.com/stretchr/testify/require"
)

// ensureDynamoTable creates this binary's certificates table if it doesn't exist, and empties it
func ensureDynamoTable(t *testing.T, rt *runtime.Runtime) {
	t.Helper()

	ctx := context.Background()
	name := rt.Config.CertsTable()

	created, err := certs.EnsureTable(ctx, rt.Dynamo, name)
	require.NoError(t, err, "error ensuring DynamoDB table")
	if created {
		return
	}

	table := aws.String(name)
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
