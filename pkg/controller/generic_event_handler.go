package controller

import (
	"github.com/atlassian/smith/pkg/util/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GenericHandler struct {
	logger       *zap.Logger
	queue        WorkQueueProducer
	zapNameField ZapNameField
}

func (g *GenericHandler) OnAdd(obj interface{}) {
	metaObj := obj.(meta_v1.Object)
	g.logger.Info("Enqueuing object because it was added", logz.NamespaceName(metaObj.GetNamespace()), g.zapNameField(metaObj.GetName()))
	g.queue.Add(QueueKey{
		Namespace: metaObj.GetNamespace(),
		Name:      metaObj.GetName(),
	})
}

func (g *GenericHandler) OnUpdate(oldObj, newObj interface{}) {
	metaObj := newObj.(meta_v1.Object)
	g.logger.Info("Enqueuing object because it was updated", logz.NamespaceName(metaObj.GetNamespace()), g.zapNameField(metaObj.GetName()))
	g.queue.Add(QueueKey{
		Namespace: metaObj.GetNamespace(),
		Name:      metaObj.GetName(),
	})
}

func (g *GenericHandler) OnDelete(obj interface{}) {
	metaObj := obj.(meta_v1.Object)
	g.logger.Info("Object was deleted", logz.NamespaceName(metaObj.GetNamespace()), g.zapNameField(metaObj.GetName()))
}
