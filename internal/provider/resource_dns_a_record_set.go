// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/miekg/dns"
)

func resourceDnsARecordSet() *schema.Resource {
	return &schema.Resource{
		CreateContext: resourceDnsARecordSetCreate,
		ReadContext:   resourceDnsARecordSetRead,
		UpdateContext: resourceDnsARecordSetUpdate,
		DeleteContext: resourceDnsARecordSetDelete,
		Importer: &schema.ResourceImporter{
			StateContext: resourceDnsImport,
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
				Description: "The IPv4 addresses this record set will point to.",
			},
			"ttl": {
				Type:        schema.TypeInt,
				Optional:    true,
				ForceNew:    true,
				Default:     3600,
				Description: "The TTL of the record set. Defaults to `3600`.",
			},
		},

		Description: "Creates an A type DNS record set.",
	}
}

func resourceDnsARecordSetCreate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	d.SetId(resourceFQDN(d))

	return resourceDnsARecordSetUpdate(ctx, d, meta)
}

func resourceDnsARecordSetRead(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	answers, err := resourceDnsRead(d, meta, dns.TypeA)
	if err != nil {
		return err
	}

	if len(answers) > 0 {

		var ttl sort.IntSlice

		addresses := schema.NewSet(hashIPString, nil)
		for _, record := range answers {
			addr, t, err := getAVal(record)
			if err != nil {
				return diag.Errorf("Error querying DNS record: %s", err)
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

func resourceDnsARecordSetUpdate(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

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
				rrStr := fmt.Sprintf("%s %d A %s", rec_fqdn, ttl, stripLeadingZeros(addr.(string)))

				rr_remove, err := dns.NewRR(rrStr)
				if err != nil {
					return diag.Errorf("error reading DNS record (%s): %s", rrStr, err)
				}

				msg.Remove([]dns.RR{rr_remove})
			}
			// Loop through all the new addresses and insert them
			for _, addr := range add {
				//nolint:forcetypeassert
				rrStr := fmt.Sprintf("%s %d A %s", rec_fqdn, ttl, stripLeadingZeros(addr.(string)))

				rr_insert, err := dns.NewRR(rrStr)
				if err != nil {
					return diag.Errorf("error reading DNS record (%s): %s", rrStr, err)
				}

				msg.Insert([]dns.RR{rr_insert})
			}

			dnsClient, ok := meta.(*DNSClient)
			if !ok {
				diag.Errorf("Error asserting meta to *DNSClient")
			}

			r, err := exchange(msg, true, dnsClient)
			if err != nil {
				d.SetId("")
				return diag.Errorf("Error updating DNS record: %s", err)
			}
			if r.Rcode != dns.RcodeSuccess {
				d.SetId("")
				return diag.Errorf("Error updating DNS record: %v (%s)", r.Rcode, dns.RcodeToString[r.Rcode])
			}
		}

		return resourceDnsARecordSetRead(ctx, d, meta)
	} else {
		return diag.Errorf("update server is not set")
	}
}

func resourceDnsARecordSetDelete(ctx context.Context, d *schema.ResourceData, meta interface{}) diag.Diagnostics {

	return resourceDnsDelete(d, meta, dns.TypeA)
}
