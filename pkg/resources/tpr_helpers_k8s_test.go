package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestExtractApiGroupAndKind(t *testing.T) {
	gk, err := ExtractApiGroupAndKind("postgresql-database-service.tpr.atlassian.com")
	require.NoError(t, err)
	assert.Equal(t, schema.GroupKind{
		Group: "tpr.atlassian.com",
		Kind:  "PostgresqlDatabaseService",
	}, gk)
}
