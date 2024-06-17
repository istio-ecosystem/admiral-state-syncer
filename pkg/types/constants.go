package types

const (
	DeploymentControllerPrefix = "deployment-ctrl"
	RolloutControllerPrefix    = "rollouts-ctrl"
	ProcessingInProgress       = "ProcessingInProgress"
	NotProcessed               = "NotProcessed"
	Processed                  = "Processed"
	Default                    = "default"
	EnvLabel                   = "env"
	Sep                        = "."
)

var (
	LogCacheFormat         = "op=%s type=%v name=%v namespace=%s cluster=%s message=%s"
	LogFormat              = "op=%v type=%v name=%v cluster=%s message=%v"
	LogFormatAdv           = "op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	LogFormatNew           = "op=%v type=%v name=%v namespace=%s identity=%s cluster=%s message=%v"
	LogFormatOperationTime = "op=%v type=%v name=%v namespace=%s cluster=%s message=%v"
	LogErrFormat           = "op=%v type=%v name=%v cluster=%v error=%v"
	AlertLogMsg            = "type assertion failed, %v is not of type string"
	AssertionLogMsg        = "type assertion failed, %v is not of type *RemoteRegistry"
)
