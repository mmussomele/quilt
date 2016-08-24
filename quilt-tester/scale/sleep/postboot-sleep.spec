var image = "mmussomele/sleep";
var numMasters = 1;
var numWorkers = 8;
var numContainers = 101;

AdminACL = ["local"]

var machineCfg = new Machine({
    provider: "Amazon",
    size: "m4.4xlarge",
    keys: githubKeys("YOUR_GITHUB_USERNAME"),
});

deployWorkers(numWorkers, machineCfg);
deployMasters(numMasters, machineCfg);

var sleepContainers = _(numContainers).times(function() {
    return new Docker(image, {args: [], env: {}});
});
