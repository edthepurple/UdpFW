# udp2xor

Takes WireGuard/OpenVPN udp packets and encrypts them with XOR so it gets through the Kirsakht firewalls of Iran's internet.

Usage:

(server side, for example hetzner where your vpn server is on port 8443)

./udp2xor -forward 0.0.0.0:8444~127.0.0.1:8443 -xor 8 -timeout 120

(client side, for example Iran)

./udp2xor -forward 0.0.0.0:8443~x.x.x.x:8444 where x.x.x.x is the ip of your germany server

if you're using wireguard, make sure to set MTU to 1280 and if you're using openvpn make sure to set

keepalive 5 120

tun-mtu 1500

mssfix 1420

in your server.conf

you can increase buffer-size up to 2048 but I do not recommend more than that because your germany server will be automatically blocked by DPI.
do not decrease the xor value below 8 or you will face packet loss.
