package templatecontroller

import (
	smith_v1 "github.com/atlassian/smith/pkg/apis/smith/v1"
	"github.com/atlassian/smith/pkg/util/logz"

	"go.uber.org/zap"
	"k8s.io/client-go/tools/cache"
)

func (c *TemplateController) onTemplateRenderAdd(obj interface{}) {
	templateRender := obj.(*smith_v1.TemplateRender)
	c.Logger.Info("Rebuilding TemplateRender because it was added", logz.Namespace(templateRender), logz.TemplateRender(templateRender))
	c.enqueue(templateRender)
}

func (c *TemplateController) onTemplateRenderUpdate(oldObj, newObj interface{}) {
	templateRender := newObj.(*smith_v1.TemplateRender)
	c.Logger.Info("Rebuilding TemplateRender because it was updated", logz.Namespace(templateRender), logz.TemplateRender(templateRender))
	c.enqueue(templateRender)
}

func (c *TemplateController) onTemplateRenderDelete(obj interface{}) {
	templateRender, ok := obj.(*smith_v1.TemplateRender)
	if !ok {
		tombstone, ok := obj.(cache.DeletedFinalStateUnknown)
		if !ok {
			c.Logger.Sugar().Errorf("Delete event with unrecognized object type: %T", obj)
			return
		}
		namespace, name, err := cache.SplitMetaNamespaceKey(tombstone.Key)
		if err != nil {
			c.Logger.
				With(zap.Error(err)).
				Sugar().Errorf("Failed to split key %q", tombstone.Key)
			c.Logger.Sugar().Infof("Rebuilding deleted TemplateRender (tombstone) with key=%q", tombstone.Key)
		} else {
			c.Logger.Info("Rebuilding deleted TemplateRender (tombstone)", logz.NamespaceName(namespace), logz.TemplateRenderName(name))
		}
		c.enqueueKey(tombstone.Key)
		return
	}
	c.Logger.Info("Rebuilding TemplateRender because it was deleted", logz.Namespace(templateRender), logz.TemplateRender(templateRender))
	c.enqueue(templateRender)
}
