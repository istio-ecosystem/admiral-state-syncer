package controller

import (
	"strings"

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

func GetDeploymentGlobalIdentifier(deployment *k8sAppsV1.Deployment) string {
	identity := deployment.Spec.Template.Labels[config.GetWorkloadIdentifier()]
	if len(identity) == 0 {
		//TODO can this be removed now? This was for backward compatibility
		identity = deployment.Spec.Template.Annotations[config.GetWorkloadIdentifier()]
	}
	if config.EnableSWAwareNSCaches() && len(identity) > 0 && len(GetDeploymentIdentityPartition(deployment)) > 0 {
		identity = GetDeploymentIdentityPartition(deployment) + "." + strings.ToLower(identity)

	}
	return identity
}

func GetDeploymentIdentityPartition(deployment *k8sAppsV1.Deployment) string {
	identityPartition := deployment.Spec.Template.Annotations[config.GetPartitionIdentifier()]
	if len(identityPartition) == 0 {
		//In case partition is accidentally applied as Label
		identityPartition = deployment.Spec.Template.Labels[config.GetPartitionIdentifier()]
	}
	return identityPartition
}
