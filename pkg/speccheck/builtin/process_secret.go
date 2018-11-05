package builtin

import (
	"github.com/atlassian/smith/pkg/speccheck"
	"github.com/atlassian/smith/pkg/util"
	core_v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

type secret struct {
}

func (secret) BeforeCreate(ctx *speccheck.Context, spec *unstructured.Unstructured) (runtime.Object /*updatedSpec*/, error) {
	return spec, nil
}

func (secret) ApplySpec(ctx *speccheck.Context, spec, actual *unstructured.Unstructured) (runtime.Object, error) {
	var secretSpec core_v1.Secret
	if err := util.ConvertType(coreV1Scheme, spec, &secretSpec); err != nil {
		return nil, err
	}

	// StringData overwrites Data
	if len(secretSpec.StringData) > 0 {
		if secretSpec.Data == nil {
			secretSpec.Data = make(map[string][]byte, len(secretSpec.StringData))
		}
		for k, v := range secretSpec.StringData {
			secretSpec.Data[k] = []byte(v)
		}
		secretSpec.StringData = nil
	}

	return &secretSpec, nil
}
