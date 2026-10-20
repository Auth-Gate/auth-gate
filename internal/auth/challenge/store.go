package challenge

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/huy090202/auth-gate/internal/platform/dynamo"
)

type dynamoStore struct {
	c     *dynamodb.Client
	table string
}

// NewDynamoStore returns a Store backed by the single table.
func NewDynamoStore(c *dynamodb.Client, table string) Store { return &dynamoStore{c: c, table: table} }

func challengeKey(clientID, id string) map[string]types.AttributeValue {
	return dynamo.Key("CH#"+clientID+"#"+id, "META")
}

func registrationKey(clientID, id string) map[string]types.AttributeValue {
	return dynamo.Key("R#"+clientID+"#"+id, "META")
}

func str(s string) types.AttributeValue { return &types.AttributeValueMemberS{Value: s} }
func num(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}
func bin(b []byte) types.AttributeValue { return &types.AttributeValueMemberB{Value: b} }

// expiry is stored as unix milliseconds so conditions can compare it to now;
// ttl is unix seconds, as DynamoDB TTL requires.
func millis(t time.Time) int64 { return t.UnixMilli() }

func isConditionFailed(err error) bool {
	var cond *types.ConditionalCheckFailedException
	return errors.As(err, &cond)
}

func (s *dynamoStore) Create(ctx context.Context, c Challenge) error {
	item := challengeKey(c.ClientID, c.ID)
	item["id"] = str(c.ID)
	item["clientId"] = str(c.ClientID)
	item["subjectId"] = str(c.SubjectID)
	item["purpose"] = str(c.Purpose)
	item["factor"] = str(string(c.Factor))
	item["status"] = str(c.Status)
	item["attempts"] = num(int64(c.Attempts))
	item["maxAttempts"] = num(int64(c.MaxAttempts))
	item["expiresAt"] = num(millis(c.ExpiresAt))
	item["createdAt"] = num(millis(c.CreatedAt))
	item["ttl"] = num(c.ExpiresAt.Unix())
	if len(c.Session) > 0 {
		item["session"] = bin(c.Session)
	}
	_, err := s.c.PutItem(ctx, &dynamodb.PutItemInput{
		TableName:           s.tableName(),
		Item:                item,
		ConditionExpression: aws.String("attribute_not_exists(pk)"),
	})
	if err != nil {
		return fmt.Errorf("create challenge: %w", err)
	}
	return nil
}

func (s *dynamoStore) Attempt(ctx context.Context, clientID, id string, now time.Time) (Challenge, error) {
	out, err := s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                s.tableName(),
		Key:                      challengeKey(clientID, id),
		UpdateExpression:         aws.String("ADD attempts :one"),
		ConditionExpression:      aws.String("#status = :pending AND attempts < maxAttempts AND expiresAt > :now"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":one": num(1), ":pending": str(StatusPending), ":now": num(millis(now)),
		},
		ReturnValues: types.ReturnValueAllNew,
	})
	if err != nil {
		if isConditionFailed(err) {
			return Challenge{}, ErrGone
		}
		return Challenge{}, fmt.Errorf("attempt challenge: %w", err)
	}
	return challengeFromItem(out.Attributes), nil
}

func (s *dynamoStore) Complete(ctx context.Context, clientID, id string, now time.Time) error {
	_, err := s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                s.tableName(),
		Key:                      challengeKey(clientID, id),
		UpdateExpression:         aws.String("SET #status = :verified"),
		ConditionExpression:      aws.String("#status = :pending AND expiresAt > :now"),
		ExpressionAttributeNames: map[string]string{"#status": "status"},
		ExpressionAttributeValues: map[string]types.AttributeValue{
			":verified": str(StatusVerified), ":pending": str(StatusPending), ":now": num(millis(now)),
		},
	})
	if err != nil {
		if isConditionFailed(err) {
			return ErrGone
		}
		return fmt.Errorf("complete challenge: %w", err)
	}
	return nil
}

func (s *dynamoStore) PutRegistration(ctx context.Context, r Registration) error {
	item := registrationKey(r.ClientID, r.ID)
	item["id"] = str(r.ID)
	item["clientId"] = str(r.ClientID)
	item["subjectId"] = str(r.SubjectID)
	item["expiresAt"] = num(millis(r.ExpiresAt))
	item["ttl"] = num(r.ExpiresAt.Unix())
	if r.DisplayName != "" {
		item["displayName"] = str(r.DisplayName)
	}
	if len(r.Session) > 0 {
		item["session"] = bin(r.Session)
	}
	if _, err := s.c.PutItem(ctx, &dynamodb.PutItemInput{TableName: s.tableName(), Item: item}); err != nil {
		return fmt.Errorf("put registration: %w", err)
	}
	return nil
}

// TakeRegistration reads the registration with a strongly consistent Get, then
// deletes it conditionally so only one concurrent caller receives it.
func (s *dynamoStore) TakeRegistration(ctx context.Context, clientID, id string, now time.Time) (Registration, error) {
	key := registrationKey(clientID, id)
	got, err := s.c.GetItem(ctx, &dynamodb.GetItemInput{TableName: s.tableName(), Key: key, ConsistentRead: aws.Bool(true)})
	if err != nil {
		return Registration{}, fmt.Errorf("get registration: %w", err)
	}
	if got.Item == nil || getN(got.Item, "expiresAt") <= millis(now) {
		return Registration{}, ErrGone
	}
	_, err = s.c.DeleteItem(ctx, &dynamodb.DeleteItemInput{
		TableName:                 s.tableName(),
		Key:                       key,
		ConditionExpression:       aws.String("attribute_exists(pk) AND expiresAt > :now"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":now": num(millis(now))},
	})
	if err != nil {
		if isConditionFailed(err) {
			return Registration{}, ErrGone
		}
		return Registration{}, fmt.Errorf("take registration: %w", err)
	}
	it := got.Item
	return Registration{
		ID:          getS(it, "id"),
		ClientID:    getS(it, "clientId"),
		SubjectID:   getS(it, "subjectId"),
		DisplayName: getS(it, "displayName"),
		Session:     getB(it, "session"),
		ExpiresAt:   time.UnixMilli(getN(it, "expiresAt")).UTC(),
	}, nil
}

func (s *dynamoStore) tableName() *string { return aws.String(s.table) }

func challengeFromItem(it map[string]types.AttributeValue) Challenge {
	return Challenge{
		ID:          getS(it, "id"),
		ClientID:    getS(it, "clientId"),
		SubjectID:   getS(it, "subjectId"),
		Purpose:     getS(it, "purpose"),
		Factor:      Factor(getS(it, "factor")),
		Session:     getB(it, "session"),
		Status:      getS(it, "status"),
		Attempts:    int(getN(it, "attempts")),
		MaxAttempts: int(getN(it, "maxAttempts")),
		ExpiresAt:   time.UnixMilli(getN(it, "expiresAt")).UTC(),
		CreatedAt:   time.UnixMilli(getN(it, "createdAt")).UTC(),
	}
}

func getS(it map[string]types.AttributeValue, k string) string {
	if v, ok := it[k].(*types.AttributeValueMemberS); ok {
		return v.Value
	}
	return ""
}

func getN(it map[string]types.AttributeValue, k string) int64 {
	if v, ok := it[k].(*types.AttributeValueMemberN); ok {
		n, _ := strconv.ParseInt(v.Value, 10, 64)
		return n
	}
	return 0
}

func getB(it map[string]types.AttributeValue, k string) []byte {
	if v, ok := it[k].(*types.AttributeValueMemberB); ok {
		return v.Value
	}
	return nil
}
