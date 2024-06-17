package types

const (
	DeploymentControllerPrefix = "deployment-ctrl"
	RolloutControllerPrefix    = "rollouts-ctrl"
	ProcessingInProgress       = "ProcessingInProgress"
	NotProcessed               = "NotProcessed"
	Processed                  = "Processed"
)

var (
	LogCacheFormat = "op=%s type=%v name=%v namespace=%s cluster=%s message=%s"
)
