// Package dynamotest provides a throwaway DynamoDB Local table for store tests.
package dynamotest

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"

	"github.com/huy090202/auth-gate/internal/platform/dynamo"
	"github.com/huy090202/auth-gate/internal/platform/id"
)

// NewTable creates a unique table on the endpoint in DYNAMO_ENDPOINT and
// deletes it when the test ends. The test is skipped when the variable is unset.
func NewTable(t *testing.T) (*dynamodb.Client, string) {
	t.Helper()
	endpoint := os.Getenv("DYNAMO_ENDPOINT")
	if endpoint == "" {
		t.Skip("DYNAMO_ENDPOINT not set; skipping DynamoDB test")
	}
	ctx := context.Background()
	c, err := dynamo.New(ctx, endpoint)
	if err != nil {
		t.Fatalf("dynamo client: %v", err)
	}
	name := "authgate-test-" + strings.ToLower(id.New())
	if err := dynamo.EnsureTable(ctx, c, name); err != nil {
		t.Fatalf("ensure table: %v", err)
	}
	t.Cleanup(func() {
		_, _ = c.DeleteTable(context.Background(), &dynamodb.DeleteTableInput{TableName: aws.String(name)})
	})
	return c, name
}
