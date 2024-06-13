package monitoring

import (
	v12 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func GenerateTxId(meta v12.Object, ctrlName string, id string) string {
	if meta != nil {
		if len(meta.GetResourceVersion()) > 0 {
			id = meta.GetResourceVersion() + "-" + id
		}
	}
	return id
}
