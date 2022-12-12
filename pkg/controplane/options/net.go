package options

import (
	"fmt"
	"net"
	"strings"

	"github.com/spf13/pflag"
	"k8s.io/kubernetes/pkg/controlplane"
	netutils "k8s.io/utils/net"
)

type NetOptions struct {
	// ServiceClusterIPRange is mapped to input provided by user
	ServiceClusterIPRanges string

	// APIServerServiceIP is the first valid IP from PrimaryServiceClusterIPRange
	APIServerServiceIP net.IP
	// PrimaryServiceClusterIPRange and SecondaryServiceClusterIPRange are the results
	// of parsing ServiceClusterIPRange into actual values
	PrimaryServiceClusterIPRange   net.IPNet
	SecondaryServiceClusterIPRange net.IPNet
}

func NewNetOptions() *NetOptions {
	return &NetOptions{}
}

func (options *NetOptions) addFlags(fs *pflag.FlagSet) {
	fs.StringVar(&options.ServiceClusterIPRanges, "service-cluster-ip-range", options.ServiceClusterIPRanges, "A CIDR notation IP range from which to assign service cluster IPs. This must not overlap with any IP ranges assigned to nodes or pods. Max of two dual-stack CIDRs is allowed.")
}

// process s.ServiceClusterIPRange from list to Primary and Secondary
// we process secondary only if provided by user
func (options *NetOptions) complete() error {
	serviceClusterIPRangeList := []string{}
	if options.ServiceClusterIPRanges != "" {
		serviceClusterIPRangeList = strings.Split(options.ServiceClusterIPRanges, ",")
	}

	if len(serviceClusterIPRangeList) == 0 {
		var primaryServiceClusterCIDR net.IPNet
		var err error
		if options.PrimaryServiceClusterIPRange, options.APIServerServiceIP, err = controlplane.ServiceIPRange(primaryServiceClusterCIDR); err != nil {
			return fmt.Errorf("error determining service IP ranges: %v", err)
		}
		options.SecondaryServiceClusterIPRange = net.IPNet{}
		return nil
	}

	_, primaryServiceClusterCIDR, err := netutils.ParseCIDRSloppy(serviceClusterIPRangeList[0])
	if err != nil {
		return fmt.Errorf("service-cluster-ip-range[0] is not a valid cidr")
	}
	if options.PrimaryServiceClusterIPRange, options.APIServerServiceIP, err = controlplane.ServiceIPRange(*primaryServiceClusterCIDR); err != nil {
		return fmt.Errorf("error determining service IP ranges for primary service cidr: %v", err)
	}

	// user provided at least two entries
	// note: validation asserts that the list is max of two dual stack entries
	if len(serviceClusterIPRangeList) > 1 {
		_, secondaryServiceClusterCIDR, err := netutils.ParseCIDRSloppy(serviceClusterIPRangeList[1])
		if err != nil {
			return fmt.Errorf("service-cluster-ip-range[1] is not an ip net")
		}
		options.SecondaryServiceClusterIPRange = *secondaryServiceClusterCIDR
	}
	return nil
}
