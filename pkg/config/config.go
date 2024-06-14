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
