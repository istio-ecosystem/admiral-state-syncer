package controller

import (
	"github.com/istio-ecosystem/admiral-state-syncer/pkg/config"

	k8sAppsV1 "k8s.io/api/apps/v1"
)

func GetDeploymentOriginalIdentifier(deployment *k8sAppsV1.Deployment) string {
	identity := deployment.Spec.Template.Labels[config.GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = deployment.Spec.Template.Annotations[config.GetWorkloadIdentifier()]
	}
	return identity
}
