containerIDCounter = 0;
ctx = {
    invariants: [],
    connections: [],
    machines: [],
    labels: [],
    placements: [],
    maxPrice: 0.0,
    namespace: "",
    adminACL: [],
};

function Machine(optionalArgs) {
    this.provider = optionalArgs.provider || "";
    this.role = optionalArgs.role || "";
    this.region = optionalArgs.region || "";
    this.size = optionalArgs.size || "";
    this.cpu = optionalArgs.cpu || new Range(0, 0);
    this.ram = optionalArgs.ram || new Range(0, 0);
    this.diskSize = optionalArgs.diskSize || 0;
    this.keys = optionalArgs.keys || [];
}

Machine.prototype.clone = function() {
    return new Machine(this);
};

Machine.prototype.withRole = function(role) {
    var copy = this.clone();
    copy.role = role;
    return copy;
};

function MachineRule(exclusive, optionalArgs) {
    this.exclusive = exclusive;
    if (optionalArgs.provider) {
        this.provider = optionalArgs.provider;
    }
    if (optionalArgs.size) {
        this.size = optionalArgs.size;
    }
    if (optionalArgs.region) {
        this.region = optionalArgs.region;
    }
}

var labelNameCount = {};
function uniqueLabelName(name) {
    if (!(name in labelNameCount)) {
        labelNameCount[name] = 0;
    }
    var count = ++labelNameCount[name];
    // XXX: Always append the count?
    if (count == 1) {
        return name;
    }
    return name + labelNameCount[name];
}

function Label(name, containers) {
    this.name = uniqueLabelName(name);
    this.containers = containers;
    this.annotations = [];
    ctx.labels.push(this);
}

Label.prototype.hostname = function() {
    return this.name + ".q";
};

Label.prototype.children = function() {
    var res = [];
    for (var i = 1; i < this.containers.length + 1; i++) {
        res.push(i + "." + this.name + ".q");
    }
    return res;
};

Label.prototype.annotate = function(annotation) {
    this.annotations.push(annotation);
    return this;
};

function LabelRule(exclusive, otherLabel) {
    this.exclusive = exclusive;
    this.otherLabel = otherLabel;
}

function Docker(image, args) {
	this.id = ++containerIDCounter;
    this.image = image;
    this.args = [];
    if (args) {
        this.args = args;
    }
    this.env = {};
}

Docker.prototype.clone = function() {
	var cloned = new Docker(this.image, this.args);
	cloned.env = this.env;
	return cloned;
}

Docker.prototype.replicate = function(n) {
	var res = [];
	for (var i = 0 ; i < n ; i++) {
		res.push(this.clone());
	}
	return res;
}

Docker.prototype.withEnv = function(env) {
	this.env = env;
	return this;
}

function Placement(target, rule) {
    this.target = target;
    this.rule = rule;
}

function Connection(ports, from, to) {
    // Box raw integers into ports.
    if (typeof ports === "number") {
        ports = new Port(ports);
    }
    this.ports = ports;
    this.from = from;
    this.to = to;
}

function Range(min, max) {
    this.min = min;
    this.max = max;
}

function Reachable(from, to) {
    this.type = "reach";
    this.from = from;
    this.to = to;
}

function ACLReachable(from, to) {
    this.type = "reachACL";
    this.from = from;
    this.to = to;
}

function Neighborship(from, to) {
    this.type = "reachDirect";
    this.from = from;
    this.to = to;
}

function Enough() {
    this.type = "enough";
}

function Between(from, between, to) {
    this.type = "between";
    this.from = from;
    this.between = between;
    this.to = to;
}

function Assertion(invariant, desired) {
    this.invariant = invariant;
    this.desired = desired;
}

var publicInternet = new Label("public", []);

function Port(p) {
    return new Range(p, p);
}

function assert(rule, desired) {
    ctx.invariants.push(new Assertion(rule, desired));
}

function connect(range, from, to) {
    var fromPublic = from.name == publicInternet.name;
    var toPublic = to.name == publicInternet.name;
    if (fromPublic && toPublic) {
        throw "cannot connect public internet to itself";
    }
    if ((fromPublic || toPublic) && (range.min != range.max)) {
        throw "public internet cannot connect on port ranges";
    }
    ctx.connections.push(new Connection(range, from, to));
}

function deployMachines(machines) {
    ctx.machines = ctx.machines.concat(machines);
}

function deployMasters(n, machine) {
    deployMachines(_(n).times(function() {
        return machine.withRole("Master");
    }));
}

function deployWorkers(n, machine) {
    deployMachines(_(n).times(function() {
        return machine.withRole("Worker");
    }));
}

function place(tgt, rule) {
    ctx.placements.push(new Placement(tgt, rule));
}

function setAdminACL(acl) {
    ctx.adminACL = acl;
}

function setNamespace(namespace) {
    ctx.namespace = namespace;
}

function setMaxPrice(maxPrice) {
    ctx.maxPrice = maxPrice;
}

PortRange = Range;
