# UDPFW

Simple UDP Forwarder

Usage:

(server side, for example hetzner where your vpn server is on port 8443)

./tunnel -from 0.0.0.0:8444 -to 127.0.0.1:8443 -timeout 5m

(client side, for example Iran)

./tunnel -from 0.0.0.0:8443 -to [ServerIP]:8444 -timeout 5m

if you're using wireguard, make sure to set MTU to 1280 and if you're using openvpn make sure to set

keepalive 5 120

tun-mtu 1500

mssfix 1420

in your server.conf
