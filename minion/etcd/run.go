package etcd

// Run synchronizes state in the Database with the Etcd cluster.
func Run() {
	store := NewStore()
	makeEtcdDir(minionDir, store, 0)
	makeEtcdDir(subnetStore, store, 0)
	makeEtcdDir(nodeStore, store, 0)

	go runElection(store)
	go runNetwork(store)
	runMinionSync(store)
}
