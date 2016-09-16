require("github.com/NetSys/quilt/quilt-tester/config/infrastructure")

// Using unique Namespaces will allow multiple Quilt instances to run on the
// same cloud provider account without conflict.
setNamespace("REPLACED_IN_TEST_RUN");

// Defines the set of addresses that are allowed to access Quilt VMs.
setNamespace(AdminACL["local"]);

var c = new Docker("alpine", ["tail", "-f", "/dev/null"]);
var red = new Label("red", c.replicate(10));
var blue = new Label("blue", c.replicate(10));
var yellow = new Label("yellow", c.replicate(10));

connect(80, red, blue)
connect(80, red, yellow)
connect(80, blue, yellow)
