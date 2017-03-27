package network

import (
	"fmt"
	"net"
	"strings"
	"sync"

	"github.com/quilt/quilt/db"
	"github.com/quilt/quilt/join"
	"github.com/quilt/quilt/minion/ipdef"

	log "github.com/Sirupsen/logrus"
	"github.com/miekg/dns"
)

const dnsTTL = 60 // Seconds

type dnsTable struct {
	server dns.Server

	recordLock sync.Mutex
	records    map[string]net.IP
}

var table *dnsTable

func runDNS(conn db.Conn) {
	conn.RegisterCallback(func() {
		syncHostnamesOnce(conn)
	}, "DNS Sync Hostnames", 0, db.LabelTable, db.ContainerTable, db.MinionTable)

	conn.RegisterCallback(func() {
		serveDNSOnce(conn)
	}, "DNS Serve", 0, db.HostnameTable, db.MinionTable)
}

func syncHostnamesOnce(conn db.Conn) {
	if !conn.EtcdLeader() {
		return
	}

	conn.Txn(db.LabelTable, db.ContainerTable, db.HostnameTable).Run(joinHostnames)
}

func joinHostnames(view db.Database) error {
	var target []db.Hostname
	for _, label := range view.SelectFromLabel(nil) {
		if label.IP != "" {
			target = append(target, db.Hostname{
				Hostname: label.Label,
				IP:       label.IP,
			})
		}
		for i, containerIP := range label.ContainerIPs {
			target = append(target, db.Hostname{
				Hostname: fmt.Sprintf("%d.%s", i+1, label.Label),
				IP:       containerIP,
			})
		}
	}
	for _, c := range view.SelectFromContainer(nil) {
		if c.Hostname != "" && c.IP != "" {
			target = append(target, db.Hostname{
				Hostname: c.Hostname,
				IP:       c.IP,
			})
		}
	}

	key := func(iface interface{}) interface{} {
		h := iface.(db.Hostname)
		h.ID = 0
		return h
	}
	_, toAdd, toDel := join.HashJoin(db.HostnameSlice(target),
		db.HostnameSlice(view.SelectFromHostname(nil)), key, key)

	for _, intf := range toDel {
		view.Remove(intf.(db.Hostname))
	}

	for _, intf := range toAdd {
		tgt := intf.(db.Hostname)
		dbHostname := view.InsertHostname()
		tgt.ID = dbHostname.ID
		view.Commit(tgt)
	}

	return nil
}

func serveDNSOnce(conn db.Conn) {
	self := conn.MinionSelf()

	if self.Role != db.Worker {
		if table == nil {
			return
		}

		err := table.server.Shutdown()
		if err != nil {
			log.WithError(err).Error("Failed to shut down DNS server")
		}
		table = nil
		return
	}

	table = updateTable(table, conn.SelectFromHostname(nil))
}

func updateTable(table *dnsTable, hostnames []db.Hostname) *dnsTable {
	records := hostnamesToDNS(hostnames)
	if table != nil {
		table.recordLock.Lock()
		table.records = records
		table.recordLock.Unlock()
		return table
	}
	table = makeTable(records)

	// There could be multiple messages depending on how listenAndServe is
	// implemented.  We don't want anyone to block, so we make a bit of a buffer.
	errChan := make(chan error, 8)
	table.server.NotifyStartedFunc = func() { errChan <- nil }
	go func() { errChan <- listenAndServe(table) }()

	if err := <-errChan; err != nil {
		log.WithError(err).Error("Failed to start DNS server")
		return nil
	}

	log.Info("Started DNS Server")
	return table
}

func (table *dnsTable) ServeDNS(w dns.ResponseWriter, req *dns.Msg) {
	defer w.Close()

	log.Debug("DNS Request: ", req)

	resp := table.genResponse(req)
	if resp == nil {
		return
	}
	log.Debug("DNS Response: ", resp)

	if err := w.WriteMsg(resp); err != nil {
		log.WithError(err).Error("Failed to send DNS response")
	}
}

func (table *dnsTable) genResponse(req *dns.Msg) *dns.Msg {
	resp := &dns.Msg{}
	if len(req.Question) != 1 {
		return resp.SetRcode(req, dns.RcodeNotImplemented)
	}
	q := req.Question[0]
	if q.Qclass != dns.ClassINET || q.Qtype != dns.TypeA {
		return resp.SetRcode(req, dns.RcodeNotImplemented)
	}

	ips := table.lookupA(q.Name)
	if len(ips) == 0 {
		// Even though the client asked for a hostname within `.q` that we know
		// nothing about, it's possible we'll learn about it in the future.  For
		// now, we'll just not respond, the client will time out, and try again
		// later.  Hopefully by then we have a response for them -- or if not,
		// eventually they'll give up.
		//
		// XXX: The above logic is correct for things in the .q domain name, but
		// we're also doing the same thing for failures to resolve external
		// hosts.  This isn't entirely correct, it would be much better to return
		// whatever upstream gave us in case of a failure.
		return nil
	}

	resp.SetReply(req)
	for _, ip := range ips {
		resp.Answer = append(resp.Answer, &dns.A{
			Hdr: dns.RR_Header{
				Name:   q.Name,
				Rrtype: dns.TypeA,
				Class:  dns.ClassINET,
				Ttl:    dnsTTL,
			},
			A: ip,
		})
	}
	return resp
}

func (table *dnsTable) lookupA(name string) []net.IP {
	if strings.HasSuffix(name, ".q.") {
		table.recordLock.Lock()
		ip := table.records[name]
		table.recordLock.Unlock()
		if ip == nil {
			return nil
		}
		return []net.IP{ip}
	}

	ipStrs, err := lookupHost(strings.TrimRight(name, "."))
	if err != nil {
		log.WithError(err).Debug("Failed to lookup external record: ", name)
		return nil
	}

	var ips []net.IP
	for _, ipStr := range ipStrs {
		if ip := net.ParseIP(ipStr); ip != nil && ip.To4() != nil {
			ips = append(ips, ip)
		}
	}
	return ips
}

func makeTable(records map[string]net.IP) *dnsTable {
	tbl := &dnsTable{
		records: records,
		server: dns.Server{
			Addr: fmt.Sprintf("%s:53", ipdef.GatewayIP),
			Net:  "udp",
		},
	}
	tbl.server.Handler = tbl
	return tbl
}

func hostnamesToDNS(hostnames []db.Hostname) map[string]net.IP {
	records := map[string]net.IP{}
	for _, hn := range hostnames {
		if ip := net.ParseIP(hn.IP); ip != nil {
			records[hn.Hostname+".q."] = ip
		}
	}
	return records
}

var listenAndServe = func(table *dnsTable) error {
	return table.server.ListenAndServe()
}

var lookupHost = net.LookupHost
