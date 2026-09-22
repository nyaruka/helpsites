package certs

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
)

// EnsureTable creates the certificates table if it doesn't exist and waits until it's usable, returning whether it
// was created. The table is simple enough that creating it on startup beats a separate provisioning step, but a
// deployment shouldn't see it happen twice: it's created with deletion protection, and a caller should make some
// noise when it was created.
func EnsureTable(ctx context.Context, client *dynamodb.Client, name string) (bool, error) {
	describe := &dynamodb.DescribeTableInput{TableName: aws.String(name)}

	out, err := client.DescribeTable(ctx, describe)
	if err == nil && out.Table.TableStatus == types.TableStatusActive {
		return false, nil
	}

	created := false
	var notFound *types.ResourceNotFoundException
	if errors.As(err, &notFound) {
		_, err = client.CreateTable(ctx, &dynamodb.CreateTableInput{
			TableName: aws.String(name),
			AttributeDefinitions: []types.AttributeDefinition{
				{AttributeName: aws.String("Key"), AttributeType: types.ScalarAttributeTypeS},
			},
			KeySchema:                 []types.KeySchemaElement{{AttributeName: aws.String("Key"), KeyType: types.KeyTypeHash}},
			BillingMode:               types.BillingModePayPerRequest,
			DeletionProtectionEnabled: aws.Bool(true),
		})
		created = err == nil

		// instances starting together race to create it, and the loser isn't wrong
		var inUse *types.ResourceInUseException
		if errors.As(err, &inUse) {
			err = nil
		}
	}
	if err != nil {
		return false, fmt.Errorf("error ensuring table %s: %w", name, err)
	}

	// whoever created it, real DynamoDB takes a while to make it usable (the local emulator doesn't)
	if err := dynamodb.NewTableExistsWaiter(client).Wait(ctx, describe, 2*time.Minute); err != nil {
		return false, fmt.Errorf("error waiting for table %s: %w", name, err)
	}
	return created, nil
}
