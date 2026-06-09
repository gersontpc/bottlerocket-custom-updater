package awsparam

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
)

type Reader struct {
	client        *ssm.Client
	parameterName string
}

func NewReader(ctx context.Context, parameterName, region string) (*Reader, error) {
	if strings.TrimSpace(parameterName) == "" {
		return nil, fmt.Errorf("SSM parameter name is required")
	}

	options := []func(*awsconfig.LoadOptions) error{}
	if region != "" {
		options = append(options, awsconfig.WithRegion(region))
	}

	cfg, err := awsconfig.LoadDefaultConfig(ctx, options...)
	if err != nil {
		return nil, err
	}

	return &Reader{
		client:        ssm.NewFromConfig(cfg),
		parameterName: parameterName,
	}, nil
}

func (r *Reader) Read(ctx context.Context) (string, error) {
	out, err := r.client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(r.parameterName),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return "", err
	}
	if out.Parameter == nil || out.Parameter.Value == nil {
		return "", fmt.Errorf("SSM parameter %q has no value", r.parameterName)
	}
	return strings.TrimSpace(*out.Parameter.Value), nil
}
