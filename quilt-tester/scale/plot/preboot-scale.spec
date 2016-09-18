setAdminACL(["local"])

gitKeys = githubKeys("mmussomele").concat(githubKeys("ejj")).concat(githubKeys("kklin"))
gitKeys = gitKeys.concat(["ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQCnPFLHGpXK+Q9L0gSs7lgXHmyK91Jn1tPipTr9s24q0+X5s8P7nChFl+Oavrgt3ju2nm9nxMcYSR7id9K5465hO1yHrtp6eS7Gn/C02OO8uWXtT96pKyW8fe34ZzwmP8ZwgCmelkI7PzyK/NOw8bbj90joByeuEnerhHlmk9ShYMqlyEqxPL4KswlJTz7ZDQzVaxDTXOHGUWsDAC4VKP5mOCIVWIj55ws5l748pO5zHWWlZH47ichQRIbMBe+b7ZcvmwJHdDT3CoakTDalghugduMk1g2Cp2i92bwdErtF+rP3cyXa3MWrlWlDZ1D9BbBoeCMZmUy8lr9kr7kCz8Yp ubuntu@ip-172-31-16-18"])

var numMasters = 1;
var machineCfg = new Machine({{
    provider: "AmazonReserved",
    size: "m4.4xlarge",
    region: "us-west-1",
    diskSize: 33,
    keys: gitKeys,
}});
deployMasters(numMasters, machineCfg);

var numWorkers = 16;
var machineCfgWorker = new Machine({{
    provider: "AmazonReserved",
    size: "m4.4xlarge",
    region: "us-west-1",
    diskSize: 32,
    keys: gitKeys,
}});
deployWorkers(numWorkers, machineCfgWorker);
