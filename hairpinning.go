package hairpinning

import (
	"context"
	"net"
	"strings"
	"github.com/coredns/coredns/plugin"
	"github.com/coredns/coredns/plugin/pkg/dnsutil"
	"github.com/miekg/dns"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

type Hairpinning struct {
	next plugin.Handler
	clientset *kubernetes.Clientset
}

func New() (*Hairpinning, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Hairpinning{clientset: clientset}, nil
}

func (k *Hairpinning) ServeDNS(ctx context.Context, w dns.ResponseWriter, r *dns.Msg) error {
	state := dnsutil.NewState(r)
	qname := state.Name()
	qtype := state.QType()

	if qtype != dns.TypeA {
		return plugin.NextOrFailure(k.Name(), k.next, ctx, w, r)
	}

	ips, err := net.LookupHost(strings.TrimSuffix(qname, "."))
	if err != nil {
		return plugin.NextOrFailure(k.Name(), k.next, ctx, w, r)
	}

	resolvedIP := ips[0] // Assume first result
	clusterIP := k.getClusterIP(resolvedIP)

	resp := new(dns.Msg)
	resp.SetReply(r)
	resp.Authoritative = true
	
	rr := &dns.A{
		Hdr: dns.RR_Header{Name: qname, Rrtype: dns.TypeA, Class: dns.ClassINET, Ttl: 60},
		A:   net.ParseIP(clusterIP),
	}
	resp.Answer = append(resp.Answer, rr)

	w.WriteMsg(resp)
	return nil
}

func (k *Hairpinning) getClusterIP(externalIP string) string {
	services, err := k.clientset.CoreV1().Services("default").List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return externalIP
	}

	for _, svc := range services.Items {
		if svc.Spec.Type == "LoadBalancer" || svc.Spec.Type == "NodePort" {
			for _, ip := range svc.Status.LoadBalancer.Ingress {
				if ip.IP == externalIP {
					return svc.Spec.ClusterIP
				}
			}
		}
	}

	return externalIP
}

func (k *Hairpinning) Name() string { return "Hairpinning" }
