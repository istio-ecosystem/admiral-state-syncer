package registry

// ClusterStore stores mapping of clusters
// and the shard they belong to
type ClusterStore interface {
	AddClusterToShard() error
	AllAllClustersToShard() error
}

// IdentityStore stores mapping of identity and
// the cluster in which resources for them need to be
// created
type IdentityStore interface {
	AddIdentityToCluster() error
	AddAllIdentitiesToCluster() error
	AddIdentityConfiguration() error
}
