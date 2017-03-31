package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestGroupKindToTprName(t *testing.T) {
	name := GroupKindToTprName(schema.GroupKind{
		Group: "tpr.atlassian.com",
		Kind:  "PostgresqlDatabaseService",
	})
	assert.Equal(t, "postgresql-database-service.tpr.atlassian.com", name)
}
