package provider

import (
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/miekg/dns"
)

func resourceDnsAAAARecordSet() *schema.Resource {
	return &schema.Resource{
		Create: resourceDnsAAAARecordSetCreate,
		Read:   resourceDnsAAAARecordSetRead,
		Update: resourceDnsAAAARecordSetUpdate,
		Delete: resourceDnsAAAARecordSetDelete,
		Importer: &schema.ResourceImporter{
			State: resourceDnsImport,
		},

		Schema: map[string]*schema.Schema{
			"zone": {
				Type:         schema.TypeString,
				Required:     true,
				ForceNew:     true,
				ValidateFunc: validateZone,
				Description: "DNS zone the record set belongs to. It must be an FQDN, that is, include the trailing " +
					"dot.",
			},
			"name": {
				Type:         schema.TypeString,
				Optional:     true,
				ForceNew:     true,
				ValidateFunc: validateName,
				Description: "The name of the record set. The `zone` argument will be appended to this value to " +
					"create the full record path.",
			},
			"addresses": {
				Type:        schema.TypeSet,
				Required:    true,
				Elem:        &schema.Schema{Type: schema.TypeString},
				Set:         hashIPString,
				Description: "The IPv6 addresses this record set will point to.",
			},
			"ttl": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Default:     3600,
				Description: "The TTL of the record set. Defaults to `3600`.",
			},
		},

		Description: "Creates an AAAA type DNS record set.",
	}
}

func resourceDnsAAAARecordSetCreate(d *schema.ResourceData, meta interface{}) error {

	d.SetId(resourceFQDN(d))

	return resourceDnsAAAARecordSetUpdate(d, meta)
}

func resourceDnsAAAARecordSetRead(d *schema.ResourceData, meta interface{}) error {

	answers, err := resourceDnsRead(d, meta, dns.TypeAAAA)
	if err != nil {
		return err
	}

	if len(answers) > 0 {

		var ttl sort.IntSlice

		addresses := schema.NewSet(hashIPString, nil)
		for _, record := range answers {
			addr, t, err := getAAAAVal(record)
			if err != nil {
				return fmt.Errorf("Error querying DNS record: %s", err)
			}
			addresses.Add(addr)
			ttl = append(ttl, t)
		}
		sort.Sort(ttl)

		//nolint:errcheck
		d.Set("addresses", addresses)
		//nolint:errcheck
		d.Set("ttl", ttl[0])
	} else {
		d.SetId("")
	}

	return nil
}

func resourceDnsAAAARecordSetUpdate(d *schema.ResourceData, meta interface{}) error {

	if meta != nil {

		//nolint:forcetypeassert
		ttl := d.Get("ttl").(int)

		rec_fqdn := resourceFQDN(d)

		msg := new(dns.Msg)

		//nolint:forcetypeassert
		msg.SetUpdate(d.Get("zone").(string))

		if d.HasChange("addresses") {
			o, n := d.GetChange("addresses")
			//nolint:forcetypeassert
			os := o.(*schema.Set)
			//nolint:forcetypeassert
			ns := n.(*schema.Set)
			remove := os.Difference(ns).List()
			add := ns.Difference(os).List()

			// Loop through all the old addresses and remove them
			for _, addr := range remove {
				//nolint:forcetypeassert
				rrStr := fmt.Sprintf("%s %d AAAA %s", rec_fqdn, ttl, stripLeadingZeros(addr.(string)))

				rr_remove, err := dns.NewRR(rrStr)
				if err != nil {
					return fmt.Errorf("error reading DNS record (%s): %s", rrStr, err)
				}

				msg.Remove([]dns.RR{rr_remove})
			}
			// Loop through all the new addresses and insert them
			for _, addr := range add {
				//nolint:forcetypeassert
				rrStr := fmt.Sprintf("%s %d AAAA %s", rec_fqdn, ttl, stripLeadingZeros(addr.(string)))

				rr_insert, err := dns.NewRR(rrStr)
				if err != nil {
					return fmt.Errorf("error reading DNS record (%s): %s", rrStr, err)
				}

				msg.Insert([]dns.RR{rr_insert})
			}

			r, err := exchange(msg, true, meta)
			if err != nil {
				d.SetId("")
				return fmt.Errorf("Error updating DNS record: %s", err)
			}
			if r.Rcode != dns.RcodeSuccess {
				d.SetId("")
				return fmt.Errorf("Error updating DNS record: %v (%s)", r.Rcode, dns.RcodeToString[r.Rcode])
			}
		}

		return resourceDnsAAAARecordSetRead(d, meta)
	} else {
		return fmt.Errorf("update server is not set")
	}
}

func resourceDnsAAAARecordSetDelete(d *schema.ResourceData, meta interface{}) error {

	return resourceDnsDelete(d, meta, dns.TypeAAAA)
}
