package factor

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb/types"

	"github.com/huy090202/auth-gate/internal/auth/pin"
	"github.com/huy090202/auth-gate/internal/platform/dynamo"
)

const batchSize = 25

type dynamoStore struct {
	c     *dynamodb.Client
	table string
}

// NewDynamoStore returns a Store backed by the single table.
func NewDynamoStore(c *dynamodb.Client, table string) Store { return &dynamoStore{c: c, table: table} }

func subjectPK(clientID, subjectID string) string { return "S#" + clientID + "#" + subjectID }

func pinKey(clientID, subjectID string) map[string]types.AttributeValue {
	return dynamo.Key(subjectPK(clientID, subjectID), "PIN")
}

func credKey(clientID, subjectID, credentialID string) map[string]types.AttributeValue {
	return dynamo.Key(subjectPK(clientID, subjectID), "WA#"+credentialID)
}

func str(s string) types.AttributeValue { return &types.AttributeValueMemberS{Value: s} }
func num(n int64) types.AttributeValue {
	return &types.AttributeValueMemberN{Value: strconv.FormatInt(n, 10)}
}
func bin(b []byte) types.AttributeValue { return &types.AttributeValueMemberB{Value: b} }
func ts(t time.Time) types.AttributeValue {
	return str(t.UTC().Format(time.RFC3339Nano))
}

func (s *dynamoStore) PutPIN(ctx context.Context, clientID, subjectID string, p PIN) error {
	item := pinKey(clientID, subjectID)
	item["hash"] = bin(p.Record.Hash)
	item["salt"] = bin(p.Record.Salt)
	item["argonMemory"] = num(int64(p.Record.Params.Memory))
	item["argonTime"] = num(int64(p.Record.Params.Time))
	item["argonThreads"] = num(int64(p.Record.Params.Threads))
	item["failedCount"] = num(int64(p.FailedCount))
	item["updatedAt"] = ts(p.UpdatedAt)
	if p.LockedUntil != nil {
		item["lockedUntil"] = ts(*p.LockedUntil)
	}
	if _, err := s.c.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: item}); err != nil {
		return fmt.Errorf("put pin: %w", err)
	}
	return nil
}

func (s *dynamoStore) GetPIN(ctx context.Context, clientID, subjectID string) (PIN, error) {
	out, err := s.c.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.table), Key: pinKey(clientID, subjectID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return PIN{}, fmt.Errorf("get pin: %w", err)
	}
	if out.Item == nil {
		return PIN{}, ErrNotFound
	}
	it := out.Item
	updated, err := getTime(it, "updatedAt")
	if err != nil {
		return PIN{}, fmt.Errorf("pin: %w", err)
	}
	locked, err := getTimePtr(it, "lockedUntil")
	if err != nil {
		return PIN{}, fmt.Errorf("pin: %w", err)
	}
	return PIN{
		Record: pin.Record{
			Hash: getB(it, "hash"),
			Salt: getB(it, "salt"),
			Params: pin.Params{
				Memory:  getU[uint32](it, "argonMemory", 32),
				Time:    getU[uint32](it, "argonTime", 32),
				Threads: getU[uint8](it, "argonThreads", 8),
			},
		},
		FailedCount: int(getN(it, "failedCount")),
		LockedUntil: locked,
		UpdatedAt:   updated,
	}, nil
}

func (s *dynamoStore) DeletePIN(ctx context.Context, clientID, subjectID string) error {
	if _, err := s.c.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(s.table), Key: pinKey(clientID, subjectID)}); err != nil {
		return fmt.Errorf("delete pin: %w", err)
	}
	return nil
}

func (s *dynamoStore) RecordPINFailure(ctx context.Context, clientID, subjectID string, maxAttempts int, lockout time.Duration, now time.Time) (int, *time.Time, error) {
	out, err := s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table),
		Key:                       pinKey(clientID, subjectID),
		UpdateExpression:          aws.String("ADD failedCount :one"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":one": num(1)},
		ReturnValues:              types.ReturnValueUpdatedNew,
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return 0, nil, ErrNotFound
		}
		return 0, nil, fmt.Errorf("record pin failure: %w", err)
	}
	count := int(getN(out.Attributes, "failedCount"))
	if count < maxAttempts {
		return count, nil, nil
	}
	until := now.Add(lockout)
	_, err = s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table),
		Key:                       pinKey(clientID, subjectID),
		UpdateExpression:          aws.String("SET lockedUntil = :until, failedCount = :zero"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":until": ts(until), ":zero": num(0)},
	})
	if err != nil {
		return 0, nil, fmt.Errorf("lock pin: %w", err)
	}
	return count, &until, nil
}

func (s *dynamoStore) ResetPINFailures(ctx context.Context, clientID, subjectID string) error {
	_, err := s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table),
		Key:                       pinKey(clientID, subjectID),
		UpdateExpression:          aws.String("SET failedCount = :zero REMOVE lockedUntil"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":zero": num(0)},
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return ErrNotFound
		}
		return fmt.Errorf("reset pin failures: %w", err)
	}
	return nil
}

func (s *dynamoStore) PutCredential(ctx context.Context, clientID, subjectID string, c Credential) error {
	item := credKey(clientID, subjectID, c.ID)
	item["credentialId"] = str(c.ID)
	item["publicKey"] = bin(c.PublicKey)
	item["signCount"] = num(int64(c.SignCount))
	item["createdAt"] = ts(c.CreatedAt)
	item["backupEligible"] = &types.AttributeValueMemberBOOL{Value: c.BackupEligible}
	item["backupState"] = &types.AttributeValueMemberBOOL{Value: c.BackupState}
	if len(c.AAGUID) > 0 {
		item["aaguid"] = bin(c.AAGUID)
	}
	if len(c.Transports) > 0 {
		item["transports"] = &types.AttributeValueMemberSS{Value: c.Transports}
	}
	if c.DisplayName != "" {
		item["displayName"] = str(c.DisplayName)
	}
	if c.LastUsedAt != nil {
		item["lastUsedAt"] = ts(*c.LastUsedAt)
	}
	if _, err := s.c.PutItem(ctx, &dynamodb.PutItemInput{TableName: aws.String(s.table), Item: item}); err != nil {
		return fmt.Errorf("put credential: %w", err)
	}
	return nil
}

func (s *dynamoStore) ListCredentials(ctx context.Context, clientID, subjectID string) ([]Credential, error) {
	creds := []Credential{}
	p := dynamodb.NewQueryPaginator(s.c, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		KeyConditionExpression:    aws.String("pk = :pk AND begins_with(sk, :wa)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str(subjectPK(clientID, subjectID)), ":wa": str("WA#")},
		ConsistentRead:            aws.Bool(true),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("list credentials: %w", err)
		}
		for _, it := range page.Items {
			c, err := credFromItem(it)
			if err != nil {
				return nil, err
			}
			creds = append(creds, c)
		}
	}
	return creds, nil
}

func (s *dynamoStore) GetCredential(ctx context.Context, clientID, subjectID, credentialID string) (Credential, error) {
	out, err := s.c.GetItem(ctx, &dynamodb.GetItemInput{TableName: aws.String(s.table), Key: credKey(clientID, subjectID, credentialID), ConsistentRead: aws.Bool(true)})
	if err != nil {
		return Credential{}, fmt.Errorf("get credential: %w", err)
	}
	if out.Item == nil {
		return Credential{}, ErrNotFound
	}
	return credFromItem(out.Item)
}

func (s *dynamoStore) UpdateCredentialUse(ctx context.Context, clientID, subjectID, credentialID string, signCount uint32, backupState bool, now time.Time) error {
	_, err := s.c.UpdateItem(ctx, &dynamodb.UpdateItemInput{
		TableName:                 aws.String(s.table),
		Key:                       credKey(clientID, subjectID, credentialID),
		UpdateExpression:          aws.String("SET signCount = :sc, backupState = :bs, lastUsedAt = :at"),
		ConditionExpression:       aws.String("attribute_exists(pk)"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":sc": num(int64(signCount)), ":bs": &types.AttributeValueMemberBOOL{Value: backupState}, ":at": ts(now)},
	})
	if err != nil {
		var cond *types.ConditionalCheckFailedException
		if errors.As(err, &cond) {
			return ErrNotFound
		}
		return fmt.Errorf("update credential use: %w", err)
	}
	return nil
}

func (s *dynamoStore) DeleteCredential(ctx context.Context, clientID, subjectID, credentialID string) error {
	if _, err := s.c.DeleteItem(ctx, &dynamodb.DeleteItemInput{TableName: aws.String(s.table), Key: credKey(clientID, subjectID, credentialID)}); err != nil {
		return fmt.Errorf("delete credential: %w", err)
	}
	return nil
}

func (s *dynamoStore) PurgeSubject(ctx context.Context, clientID, subjectID string) error {
	var keys []types.WriteRequest
	p := dynamodb.NewQueryPaginator(s.c, &dynamodb.QueryInput{
		TableName:                 aws.String(s.table),
		KeyConditionExpression:    aws.String("pk = :pk"),
		ExpressionAttributeValues: map[string]types.AttributeValue{":pk": str(subjectPK(clientID, subjectID))},
		ProjectionExpression:      aws.String("pk, sk"),
		ConsistentRead:            aws.Bool(true),
	})
	for p.HasMorePages() {
		page, err := p.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("purge subject: query: %w", err)
		}
		for _, it := range page.Items {
			keys = append(keys, types.WriteRequest{DeleteRequest: &types.DeleteRequest{Key: map[string]types.AttributeValue{"pk": it["pk"], "sk": it["sk"]}}})
		}
	}
	for start := 0; start < len(keys); start += batchSize {
		end := min(start+batchSize, len(keys))
		pending := map[string][]types.WriteRequest{s.table: keys[start:end]}
		for attempt := 0; len(pending) > 0; attempt++ {
			if attempt >= 5 {
				return errors.New("purge subject: unprocessed deletes after retries")
			}
			out, err := s.c.BatchWriteItem(ctx, &dynamodb.BatchWriteItemInput{RequestItems: pending})
			if err != nil {
				return fmt.Errorf("purge subject: batch delete: %w", err)
			}
			pending = out.UnprocessedItems
			if len(pending) > 0 {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-time.After(time.Duration(attempt+1) * 50 * time.Millisecond):
				}
			}
		}
	}
	return nil
}

func credFromItem(it map[string]types.AttributeValue) (Credential, error) {
	created, err := getTime(it, "createdAt")
	if err != nil {
		return Credential{}, fmt.Errorf("credential: %w", err)
	}
	used, err := getTimePtr(it, "lastUsedAt")
	if err != nil {
		return Credential{}, fmt.Errorf("credential: %w", err)
	}
	return Credential{
		ID:             getS(it, "credentialId"),
		PublicKey:      getB(it, "publicKey"),
		SignCount:      getU[uint32](it, "signCount", 32),
		AAGUID:         getB(it, "aaguid"),
		Transports:     getSS(it, "transports"),
		DisplayName:    getS(it, "displayName"),
		BackupEligible: getBool(it, "backupEligible"),
		BackupState:    getBool(it, "backupState"),
		CreatedAt:      created,
		LastUsedAt:     used,
	}, nil
}

func getBool(it map[string]types.AttributeValue, k string) bool {
	if v, ok := it[k].(*types.AttributeValueMemberBOOL); ok {
		return v.Value
	}
	return false
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

func getSS(it map[string]types.AttributeValue, k string) []string {
	if v, ok := it[k].(*types.AttributeValueMemberSS); ok {
		return v.Value
	}
	return nil
}

func getTime(it map[string]types.AttributeValue, k string) (time.Time, error) {
	t, err := time.Parse(time.RFC3339Nano, getS(it, k))
	if err != nil {
		return time.Time{}, fmt.Errorf("bad %s: %w", k, err)
	}
	return t, nil
}

func getTimePtr(it map[string]types.AttributeValue, k string) (*time.Time, error) {
	if _, ok := it[k]; !ok {
		return nil, nil
	}
	t, err := getTime(it, k)
	if err != nil {
		return nil, err
	}
	return &t, nil
}

// getU reads an unsigned number that fits in bits; a missing or out-of-range
// value reads as 0.
func getU[T uint8 | uint32](it map[string]types.AttributeValue, k string, bits int) T {
	if v, ok := it[k].(*types.AttributeValueMemberN); ok {
		n, err := strconv.ParseUint(v.Value, 10, bits)
		if err != nil {
			return 0
		}
		return T(n) //nolint:gosec // ParseUint with bits already bounds n to T
	}
	return 0
}
