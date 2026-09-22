package certs_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/nyaruka/helpsites/v26/core/certs"
	"github.com/nyaruka/helpsites/v26/testsuite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEnsureTable(t *testing.T) {
	ctx, rt := testsuite.Runtime(t)

	// the suite's table already exists
	created, err := certs.EnsureTable(ctx, rt.Dynamo, rt.Config.CertsTable())
	require.NoError(t, err)
	assert.False(t, created)

	// a scratch table of our own, which is deletion protected so has to be unprotected to be removed
	name := rt.Config.DynamoTablePrefix + "Scratch"
	deleteTable := func() {
		_, err := rt.Dynamo.UpdateTable(ctx, &dynamodb.UpdateTableInput{
			TableName: aws.String(name), DeletionProtectionEnabled: aws.Bool(false),
		})
		require.NoError(t, err)
		_, err = rt.Dynamo.DeleteTable(ctx, &dynamodb.DeleteTableInput{TableName: aws.String(name)})
		require.NoError(t, err)
	}
	if _, err := rt.Dynamo.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(name)}); err == nil {
		deleteTable() // left by a previous run that didn't get to clean up
	}
	defer deleteTable()

	created, err = certs.EnsureTable(ctx, rt.Dynamo, name)
	require.NoError(t, err)
	assert.True(t, created)

	desc, err := rt.Dynamo.DescribeTable(ctx, &dynamodb.DescribeTableInput{TableName: aws.String(name)})
	require.NoError(t, err)
	assert.True(t, *desc.Table.DeletionProtectionEnabled)
	assert.Equal(t, "Key", *desc.Table.KeySchema[0].AttributeName)

	created, err = certs.EnsureTable(ctx, rt.Dynamo, name)
	require.NoError(t, err)
	assert.False(t, created)

	// a table that can't be created is an error
	_, err = certs.EnsureTable(ctx, rt.Dynamo, "")
	assert.Error(t, err)
}
