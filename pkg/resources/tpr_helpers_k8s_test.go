package resources

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	extensions "k8s.io/client-go/pkg/apis/extensions/v1beta1"
)

func TestExtractApiGroupAndKind(t *testing.T) {
	gk, err := ExtractApiGroupAndKind(&extensions.ThirdPartyResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "postgresql-database-service.tpr.atlassian.com",
		},
	})
	require.NoError(t, err)
	assert.Equal(t, schema.GroupKind{
		Group: "tpr.atlassian.com",
		Kind:  "PostgresqlDatabaseService",
	}, gk)
}
