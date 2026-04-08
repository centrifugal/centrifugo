package configtypes

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestKafkaConsumerConfigValidate_AssumeRoleARN(t *testing.T) {
	base := KafkaConsumerConfig{
		Brokers:       []string{"localhost:9092"},
		Topics:        []string{"t"},
		ConsumerGroup: "g",
	}

	t.Run("assume_role_arn with aws-msk-iam is valid", func(t *testing.T) {
		c := base
		c.SASLMechanism = "aws-msk-iam"
		c.AssumeRoleARN = "arn:aws:iam::123456789012:role/MSKAccess"
		require.NoError(t, c.Validate())
	})

	t.Run("assume_role_arn without aws-msk-iam is invalid", func(t *testing.T) {
		c := base
		c.SASLMechanism = "plain"
		c.AssumeRoleARN = "arn:aws:iam::123456789012:role/MSKAccess"
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "assume_role_arn requires sasl_mechanism aws-msk-iam")
	})

	t.Run("assume_role_arn with empty sasl_mechanism is invalid", func(t *testing.T) {
		c := base
		c.AssumeRoleARN = "arn:aws:iam::123456789012:role/MSKAccess"
		err := c.Validate()
		require.Error(t, err)
		require.Contains(t, err.Error(), "assume_role_arn requires sasl_mechanism aws-msk-iam")
	})
}
