package controller

import (
	"github.com/atlassian/smith/pkg/util/logz"
	"go.uber.org/zap"
	meta_v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type GenericHandler struct {
	Logger       *zap.Logger
	Queue        WorkQueueProducer
	ZapNameField ZapNameField
}

func (g *GenericHandler) OnAdd(obj interface{}) {
	metaObj := obj.(meta_v1.Object)
	g.Logger.Info("Enqueuing object because it was added", logz.NamespaceName(metaObj.GetNamespace()), g.ZapNameField(metaObj.GetName()))
	g.Queue.Add(QueueKey{
		Namespace: metaObj.GetNamespace(),
		Name:      metaObj.GetName(),
	})
}

func (g *GenericHandler) OnUpdate(oldObj, newObj interface{}) {
	metaObj := newObj.(meta_v1.Object)
	g.Logger.Info("Enqueuing object because it was updated", logz.NamespaceName(metaObj.GetNamespace()), g.ZapNameField(metaObj.GetName()))
	g.Queue.Add(QueueKey{
		Namespace: metaObj.GetNamespace(),
		Name:      metaObj.GetName(),
	})
}

func (g *GenericHandler) OnDelete(obj interface{}) {
	metaObj := obj.(meta_v1.Object)
	g.Logger.Info("Object was deleted", logz.NamespaceName(metaObj.GetNamespace()), g.ZapNameField(metaObj.GetName()))
}
