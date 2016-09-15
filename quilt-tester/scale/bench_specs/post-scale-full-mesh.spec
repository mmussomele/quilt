var image = "mmussomele/sleep";
var numContainers = {} + 1;

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

function createContainers(n) {{
    var sleepContainers = _(numContainers).times(function () {{
        return new Docker(image);
    }});
    
    return new Label(_.uniqueId("sleep-wk"), sleepContainers);
}}

allPorts = new PortRange(1000, 65535);
workers = createContainers(numContainers);
connect(allPorts, workers, workers);