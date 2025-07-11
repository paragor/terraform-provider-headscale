---
# generated by https://github.com/hashicorp/terraform-plugin-docs
page_title: "headscale_api_key Resource - headscale"
subcategory: ""
description: |-
  The api key resource allows you to make a api calls to headscale as admin
---

# headscale_api_key (Resource)

The api key resource allows you to make a api calls to headscale as admin



<!-- schema generated by tfplugindocs -->
## Schema

### Optional

- `expiration` (String) expiration of api key
- `expired` (Boolean) expiration of api key
- `ttl` (String) The time until the key expires. Valid time units are "ns", "us" (or "µs"), "ms", "s", "m", "h". Defaults to "2160h" that equal 90 days

### Read-Only

- `created_at` (String) time of creation api key
- `id` (String) ID of resources
- `key` (String, Sensitive) The api key.
