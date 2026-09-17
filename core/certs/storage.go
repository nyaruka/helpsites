package certs

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/feature/dynamodb/attributevalue"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"
	"github.com/caddyserver/certmagic"
)

// DynamoStorage keeps CertMagic's certificates, keys and ACME account in a DynamoDB table, so that every instance of
// a deployment shares them and a redeploy keeps every certificate. The table has a single string partition key,
// Key, holding CertMagic's slash-separated storage keys; listing a "directory" is a scan on the prefix, which is fine
// for a table that holds a handful of items per domain.
//
// Locks are items too, under locks/, taken with a conditional put and given an expiry so that a lock left behind by
// a dead instance is reclaimable.
type DynamoStorage struct {
	client *dynamodb.Client
	table  string
}

const (
	lockPrefix    = "locks/"
	lockDuration  = 5 * time.Minute
	lockPollEvery = time.Second
)

type storageItem struct {
	Key      string `dynamodbav:"Key"`
	Value    []byte `dynamodbav:"Value,omitempty"`
	Modified string `dynamodbav:"Modified,omitempty"`
	Expires  int64  `dynamodbav:"Expires,omitempty"` // for locks, as a unix timestamp
}

// NewDynamoStorage creates a new storage on the given table
func NewDynamoStorage(client *dynamodb.Client, table string) *DynamoStorage {
	return &DynamoStorage{client: client, table: table}
}

func (s *DynamoStorage) Store(ctx context.Context, key string, value []byte) error {
	item, err := attributevalue.MarshalMap(&storageItem{Key: key, Value: value, Modified: time.Now().UTC().Format(time.RFC3339Nano)})
	if err != nil {
		return err
	}
	_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: item})
	return err
}

func (s *DynamoStorage) Load(ctx context.Context, key string) ([]byte, error) {
	item, err := s.get(ctx, key)
	if err != nil {
		return nil, err
	}
	if item == nil {
		return nil, fs.ErrNotExist
	}
	return item.Value, nil
}

func (s *DynamoStorage) Delete(ctx context.Context, key string) error {
	keys, err := s.keysWithPrefix(ctx, key+"/")
	if err != nil {
		return err
	}
	for _, k := range append(keys, key) {
		_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(s.table), Key: s.keyOf(k)})
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *DynamoStorage) Exists(ctx context.Context, key string) bool {
	item, err := s.get(ctx, key)
	if err != nil {
		return false
	}
	if item != nil {
		return true
	}
	keys, err := s.keysWithPrefix(ctx, key+"/")
	return err == nil && len(keys) > 0
}

func (s *DynamoStorage) List(ctx context.Context, path string, recursive bool) ([]string, error) {
	prefix := path + "/"
	keys, err := s.keysWithPrefix(ctx, prefix)
	if err != nil {
		return nil, err
	}
	if recursive {
		return keys, nil
	}

	// only the immediate children, each once
	seen := make(map[string]bool)
	children := make([]string, 0, len(keys))
	for _, k := range keys {
		rest := strings.TrimPrefix(k, prefix)
		if i := strings.Index(rest, "/"); i >= 0 {
			rest = rest[:i]
		}
		child := prefix + rest
		if !seen[child] {
			seen[child] = true
			children = append(children, child)
		}
	}
	return children, nil
}

func (s *DynamoStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	item, err := s.get(ctx, key)
	if err != nil {
		return certmagic.KeyInfo{}, err
	}
	if item != nil {
		modified, _ := time.Parse(time.RFC3339Nano, item.Modified)
		return certmagic.KeyInfo{Key: key, Modified: modified, Size: int64(len(item.Value)), IsTerminal: true}, nil
	}

	keys, err := s.keysWithPrefix(ctx, key+"/")
	if err != nil {
		return certmagic.KeyInfo{}, err
	}
	if len(keys) == 0 {
		return certmagic.KeyInfo{}, fs.ErrNotExist
	}
	return certmagic.KeyInfo{Key: key, IsTerminal: false}, nil
}

// Lock takes the named lock, waiting for it if another instance holds it, until the context is done
func (s *DynamoStorage) Lock(ctx context.Context, name string) error {
	for {
		now := time.Now()
		item, err := attributevalue.MarshalMap(&storageItem{Key: lockPrefix + name, Expires: now.Add(lockDuration).Unix()})
		if err != nil {
			return err
		}

		_, err = s.client.PutItem(ctx, &dynamodb.PutItemInput{
			TableName:                 aws.String(s.table),
			Item:                      item,
			ConditionExpression:       aws.String("attribute_not_exists(#k) OR #e < :now"),
			ExpressionAttributeNames:  map[string]string{"#k": "Key", "#e": "Expires"},
			ExpressionAttributeValues: map[string]types.AttributeValue{":now": &types.AttributeValueMemberN{Value: fmt.Sprint(now.Unix())}},
		})
		if err == nil {
			return nil
		}

		var failed *types.ConditionalCheckFailedException
		if !errors.As(err, &failed) {
			return err
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(lockPollEvery):
		}
	}
}

func (s *DynamoStorage) Unlock(ctx context.Context, name string) error {
	_, err := s.client.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(s.table), Key: s.keyOf(lockPrefix + name)})
	return err
}

func (s *DynamoStorage) keyOf(key string) map[string]types.AttributeValue {
	return map[string]types.AttributeValue{"Key": &types.AttributeValueMemberS{Value: key}}
}

// get fetches the item at the given key, or nil if there isn't one
func (s *DynamoStorage) get(ctx context.Context, key string) (*storageItem, error) {
	out, err := s.client.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.table), Key: s.keyOf(key), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return nil, err
	}
	if out.Item == nil {
		return nil, nil
	}
	item := &storageItem{}
	if err := attributevalue.UnmarshalMap(out.Item, item); err != nil {
		return nil, err
	}
	return item, nil
}

// keysWithPrefix scans for the keys of every item whose key begins with the given prefix
func (s *DynamoStorage) keysWithPrefix(ctx context.Context, prefix string) ([]string, error) {
	keys := make([]string, 0, 10)

	paginator := dynamodb.NewScanPaginator(s.client, &dynamodb.ScanInput{
		TableName:                 aws.String(s.table),
		FilterExpression:          aws.String("begins_with(#k, :prefix)"),
		ProjectionExpression:      aws.String("#k"),
		ExpressionAttributeNames:  map[string]string{"#k": "Key"},
		ExpressionAttributeValues: map[string]types.AttributeValue{":prefix": &types.AttributeValueMemberS{Value: prefix}},
		ConsistentRead:            aws.Bool(true),
	})
	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return nil, err
		}
		for _, item := range page.Items {
			if k, ok := item["Key"].(*types.AttributeValueMemberS); ok {
				keys = append(keys, k.Value)
			}
		}
	}
	return keys, nil
}

var _ certmagic.Storage = (*DynamoStorage)(nil)
