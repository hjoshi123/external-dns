/*
Copyright 2023 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package aws

import (
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockSTSClient struct {
	AssumeRoleFunc func(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error)
}

func (m *mockSTSClient) AssumeRole(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
	return m.AssumeRoleFunc(ctx, params, optFns...)
}

func Test_newV2Config(t *testing.T) {
	t.Run("should use profile from credentials file", func(t *testing.T) {
		// setup
		credsFile, err := prepareCredentialsFile(t)
		defer os.Remove(credsFile.Name())
		require.NoError(t, err)
		os.Setenv("AWS_SHARED_CREDENTIALS_FILE", credsFile.Name())
		defer os.Unsetenv("AWS_SHARED_CREDENTIALS_FILE")

		// when
		cfgs, err := newV2Config(AWSSessionConfig{Profile: "profile2"}, nil)
		require.NoError(t, err)

		assert.GreaterOrEqual(t, len(cfgs), 1)
		cfg := cfgs[0]

		creds, err := cfg.Config.Credentials.Retrieve(context.Background())

		// then
		assert.NoError(t, err)
		assert.Equal(t, "AKID2345", creds.AccessKeyID)
		assert.Equal(t, "SECRET2", creds.SecretAccessKey)
	})

	t.Run("should respect env variables without profile", func(t *testing.T) {
		// setup
		os.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "topsecret")
		defer os.Unsetenv("AWS_ACCESS_KEY_ID")
		defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		// when
		cfgs, err := newV2Config(AWSSessionConfig{}, nil)
		require.NoError(t, err)
		assert.GreaterOrEqual(t, len(cfgs), 1)
		cfg := cfgs[0]

		creds, err := cfg.Config.Credentials.Retrieve(context.Background())

		// then
		assert.NoError(t, err)
		assert.Equal(t, "AKIAIOSFODNN7EXAMPLE", creds.AccessKeyID)
		assert.Equal(t, "topsecret", creds.SecretAccessKey)
	})

	t.Run("should use roles for different domains", func(t *testing.T) {
		os.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "topsecret")
		defer os.Unsetenv("AWS_ACCESS_KEY_ID")
		defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		roles := make([]string, 0)
		mockClient := &mockSTSClient{
			AssumeRoleFunc: func(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
				roles = append(roles, aws.ToString(params.RoleArn))
				return &sts.AssumeRoleOutput{
					Credentials: &types.Credentials{
						AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
						SecretAccessKey: aws.String("topsecret"),
						SessionToken:    aws.String("session-token"),
						Expiration:      aws.Time(time.Now().Add(1 * time.Hour)),
					},
				}, nil
			},
		}

		cfgs, err := newV2Config(AWSSessionConfig{
			DomainRolesMap: map[string]string{
				"example.com": "arn:aws:iam::123456789012:role/role1",
				"example.org": "arn:aws:iam::123456789012:role/role2",
			},
		}, mockClient)

		for _, cfg := range cfgs {
			_, err := cfg.Config.Credentials.Retrieve(context.Background())
			require.NoError(t, err)
		}

		require.NoError(t, err)
		assert.Contains(t, roles, "arn:aws:iam::123456789012:role/role1")
		assert.Contains(t, roles, "arn:aws:iam::123456789012:role/role2")
		assert.NotNil(t, cfgs, "expected at least one config")
	})

	t.Run("should use assume role", func(t *testing.T) {
		// setup
		os.Setenv("AWS_ACCESS_KEY_ID", "AKIAIOSFODNN7EXAMPLE")
		os.Setenv("AWS_SECRET_ACCESS_KEY", "topsecret")
		defer os.Unsetenv("AWS_ACCESS_KEY_ID")
		defer os.Unsetenv("AWS_SECRET_ACCESS_KEY")

		roles := make([]string, 0)
		mockClient := &mockSTSClient{
			AssumeRoleFunc: func(ctx context.Context, params *sts.AssumeRoleInput, optFns ...func(*sts.Options)) (*sts.AssumeRoleOutput, error) {
				roles = append(roles, aws.ToString(params.RoleArn))
				return &sts.AssumeRoleOutput{
					Credentials: &types.Credentials{
						AccessKeyId:     aws.String("AKIAIOSFODNN7EXAMPLE"),
						SecretAccessKey: aws.String("topsecret"),
						SessionToken:    aws.String("session-token"),
						Expiration:      aws.Time(time.Now().Add(1 * time.Hour)),
					},
				}, nil
			},
		}

		cfgs, err := newV2Config(AWSSessionConfig{
			AssumeRole: "arn:aws:iam::123456789012:role/role1",
		}, mockClient)

		for _, cfg := range cfgs {
			_, err := cfg.Config.Credentials.Retrieve(context.Background())
			require.NoError(t, err)
		}

		require.NoError(t, err)
		assert.Contains(t, roles, "arn:aws:iam::123456789012:role/role1")
		assert.NotNil(t, cfgs, "expected at least one config")
	})
}

func prepareCredentialsFile(t *testing.T) (*os.File, error) {
	credsFile, err := os.CreateTemp("", "aws-*.creds")
	require.NoError(t, err)
	_, err = credsFile.WriteString("[profile1]\naws_access_key_id=AKID1234\naws_secret_access_key=SECRET1\n\n[profile2]\naws_access_key_id=AKID2345\naws_secret_access_key=SECRET2\n")
	require.NoError(t, err)
	err = credsFile.Close()
	require.NoError(t, err)
	return credsFile, err
}
