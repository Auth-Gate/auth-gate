// Package ssmx reads parameters from AWS Systems Manager Parameter Store.
package ssmx

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

// GetSecureString returns the decrypted value of the parameter name.
func GetSecureString(ctx context.Context, cfg aws.Config, name string) (string, error) {
	out, err := ssm.NewFromConfig(cfg).GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(name),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", fmt.Errorf("ssmx: get %s: %w", name, err)
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("ssmx: parameter %s is empty", name)
	}
	return *out.Parameter.Value, nil
}
