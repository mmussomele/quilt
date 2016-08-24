var image = "mmussomele/sleep";
var numMasters = 1;
var numWorkers = 8;
var numContainers = 100;

AdminACL = ["local"]

var machineCfg = new Machine({
    provider: "Amazon",
    size: "m4.4xlarge",
    keys: githubKeys("YOUR_GITHUB_USERNAME"),
});

deployWorkers(numWorkers, machineCfg);
deployMasters(numMasters, machineCfg);

function createContainers(n) {
    var sleepContainers = _(numContainers).times(function () {
        return new Docker(image, {args: [], env: {}})
    });
    
    return new Label(_.uniqueId("sleep-wk"), sleepContainers);
}

allPorts = new PortRange(1000, 65535);
workers = createContainers(numContainers);
connect(allPorts, workers, workers);
