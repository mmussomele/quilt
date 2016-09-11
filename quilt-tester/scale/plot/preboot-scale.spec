setAdminACL(["local"])

gitKeys = githubKeys("mmussomele").concat(githubKeys("ejj")).concat(githubKeys("kklin"))

var numMasters = 1;
var machineCfgMaster = new Machine({{
    provider: "AmazonReserved",
    size: "m4.4xlarge",
    region: "us-west-1",
    diskSize: 64,
    keys: allKeys,
}});
deployMasters(numMasters, machineCfgMaster);

var numWorkersReserved = 9;
var machineCfgWorkerReserved = new Machine({{
    provider: "AmazonReserved",
    size: "m4.4xlarge",
    region: "us-west-1",
    diskSize: 32,
    keys: allKeys,
}});
deployWorkers(numWorkersReserved, machineCfgWorkerReserved);

var numWorkersSpot = 7;
var machineCfgWorkerSpot = new Machine({{
    provider: "AmazonSpot",
    size: "m4.4xlarge",
    region: "us-west-1",
    diskSize: 32,
    keys: allKeys,
}});
deployWorkers(numWorkersSpot, machineCfgWorkerSpot);
