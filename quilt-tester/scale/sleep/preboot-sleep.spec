var numMasters = 1;
var numWorkers = 8;

AdminACL = ["local"]

var machineCfg = new Machine({
    provider: "Amazon",
    size: "m4.4xlarge",
    keys: githubKeys("YOUR_GITHUB_USERNAME"),
});

deployWorkers(numWorkers, machineCfg);
deployMasters(numMasters, machineCfg);
