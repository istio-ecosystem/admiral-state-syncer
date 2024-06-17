package config

// Singleton
var wrapper = paramsWrapper{
	params: Params{},
}

func GetWorkloadIdentifier() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.WorkloadIdentityKey
}

func EnableSWAwareNSCaches() bool {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.EnableSWAwareNSCaches
}

func GetPartitionIdentifier() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.IdentityPartitionKey
}

func GetLabelSet() *LabelSet {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet
}

func GetEnvKey() string {
	wrapper.RLock()
	defer wrapper.RUnlock()
	return wrapper.params.LabelSet.EnvKey
}
