# Terraform Provider Headscale

Terraform provider for [Headscale](https://headscale.net/stable/).

Provide resources via grpc protocol. See [grpc_listen_addr](https://github.com/juanfont/headscale/blob/v0.26.1/config-example.yaml#L33)

# 

# TLS proxy
For better security highly recommend to use proxy with mtls and stay headscale use 127.0.0.1 as host network. 

`! Danger !` access via socket to headscale give you admin access without any token check
For example caddy:
```Caddyfile
{
    auto_https off
    auto_https disable_redirects
}

https://headscale.my.domain:50443 {
    # if it is need same port
    # bind <public_ip>
    log
    tls /opt/caddy/chain.pem /opt/caddy/private.pem {
        protocols tls1.3
        client_auth {
            mode require_and_verify
            trusted_ca_cert_file /opt/caddy/ca_cert.pem
            # if it is need to restrict access for specific user
            # trusted_leaf_cert_file /opt/caddy/local_leaf.pem
            # trusted_leaf_cert_file /opt/caddy/local_leaf_another.pem
        }
    }
    reverse_proxy unix+h2c//var/run/headscale/headscale.sock
}
```
and now you can configure provider, for example via env:
```bash
export HEADSCALE_TLS_CA_PATH=/Users/MYUSER/.pki/ca.pem
export HEADSCALE_TLS_CLIENT_CERT_PATH=/Users/MYUSER/.pki/chain.pem
export HEADSCALE_TLS_CLIENT_KEY_PATH=/Users/MYUSER/.pki/key.pem 
export HEADSCALE_ENDPOINT=headscale.my.domain:50443

terraform init
// ...
```
