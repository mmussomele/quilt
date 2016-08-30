setAdminACL(["local"])

gitKeys = githubKeys("mmussomele").concat(githubKeys("ejj")).concat(githubKeys("kklin"))

var numMasters = 1;
var numWorkers = 16;
var machineCfg = new Machine({{
    provider: "AmazonReserved",
    size: "m4.4xlarge",
    region: "us-west-1",
    keys: allKeys,
}});
deployMasters(numMasters, machineCfg);
deployWorkers(numWorkers, machineCfg);
