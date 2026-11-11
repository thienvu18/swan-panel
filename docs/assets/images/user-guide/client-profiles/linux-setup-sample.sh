#!/usr/bin/env bash
# ==============================================================================
# SwanPanel Native Linux Client Installer
# Target Server: swan-panel | Device: david-iphone-16-pro
# ==============================================================================
set -eo pipefail

if [ "$EUID" -ne 0 ]; then
  echo "[-] Please run as root: sudo bash $0"
  exit 1
fi

echo "[*] Installing SwanPanel native IKEv2 profile for 'david-iphone-16-pro'..."

# Ensure strongSwan / swanctl directories exist
mkdir -p /etc/swanctl/x509ca /etc/swanctl/x509 /etc/swanctl/rsa /etc/swanctl/conf.d

# 1. Write Root CA certificate
cat << 'EOF_CA' > /etc/swanctl/x509ca/swanpanel-ca.crt
-----BEGIN CERTIFICATE-----
MIIF1jCCA76gAwIBAgIRAOhZ9LLpj1HkJL2qxp3TSqgwDQYJKoZIhvcNAQELBQAw
gYIxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T
YW4gRnJhbmNpc2NvMSEwHwYDVQQKExhTd2FuUGFuZWwgUm9vdCBBdXRob3JpdHkx
IzAhBgNVBAMTGlN3YW5QYW5lbCBJbnRlcm5hbCBSb290IENBMB4XDTI2MTExMTAx
NTA0MFoXDTM2MTExMTAyMDA0MFowgYIxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpD
YWxpZm9ybmlhMRYwFAYDVQQHEw1TYW4gRnJhbmNpc2NvMSEwHwYDVQQKExhTd2Fu
UGFuZWwgUm9vdCBBdXRob3JpdHkxIzAhBgNVBAMTGlN3YW5QYW5lbCBJbnRlcm5h
bCBSb290IENBMIICIjANBgkqhkiG9w0BAQEFAAOCAg8AMIICCgKCAgEAuiBdWazH
d7zjuffYOXUyNBRlMj3lheTyUwxxA3sBtmIWfy91c2gcUNPRGBGmEyGtIdFhxM2M
xxp9LteHEXASLCvumAg7FwFMUzMnyr6mel7lnHFHeo1fMHY7eziajWvyBzHDZ8lS
4o2nXcEPUA7inpX2DHdWjsC9/rAKDYGC/ZrIvOl1XRmfrWt7v5VWaB92r4JSPsDR
ZH9pxrMc71Tr6AbfnDSDCAfAK7JFF5eRdR54sAL7JL4uzesLZytKhN4l+aTVgrzH
0SP3B3R33fcrt+M7pLoWu/r77ZZneh4PowCVACJpD95kxQTyq0YffWlgh6LowIQp
2bYAQnfyjMj5KACk7fAowBGPI4tea6QHadvhsHR7aEXpbUyGV4Gmmi2Mle2Wkpa1
o9ryDktUDyL5jHxpQUhBQJpMmgSFAjJmz1cvTooa1QxRhSuZL9X1JI5gsk5oVc/I
qpA7n2219M9QTAmZZ70cEd9svXDzXQILFXkGKLaVQov+MmVcodNmQXilHYNbvC+M
F0py9MYF9KekLAMB932TjZAGNgH5kXGI5uv0KbYfoks8W+kWdE3s04i21tLWUgJf
DVOiQFMCQ3YPVb+qJ6YnH69sUyq0d/KgdFzkDMTtXcbuq0sRh8m++wjAaP2YVvhC
IsFc+bQ/Up7tNJLxR7v8vKkmpODpRI6rJCECAwEAAaNFMEMwDgYDVR0PAQH/BAQD
AgGGMBIGA1UdEwEB/wQIMAYBAf8CAQEwHQYDVR0OBBYEFPZfJWwm26w+UbDA4pz1
ZzzSIzevMA0GCSqGSIb3DQEBCwUAA4ICAQBXSqmQBZskVbprdH9xJf3mCZ5HG9Hm
WREky7+Pz5wag4NXa/2lIZ42M4/6u76ogiLFFJ0PXOktU9I7STZIOGREOb/T8RCp
7itgs/rHHKKqwJfQSoxZBHJ4rnXhDYMETbp0oS2mIVinICdrWgttffyKVgYiWgy7
sJtAL0dVJGtngJP9taeSP/U2ro0zt4WUS9d5QJys9e19aUZOrGKQG8K++I6jLEmI
YEwjS3uJ5dwYaR6FsjNC5XF02pC6l0eX7wy+vHxpK12I6HKYgMoijDUwTgKWkS+t
sBQGAJL2GqJkykxTGQYhDeuiSK87VppcMewmV6awup7YF6dEIxhZW+2YF2skSxqC
fBuTjslF4+nl3OOtl2oPnWsMt+cy1uZOj19m/5XF2ohTIyfmFJBdwqGuvQRd4EQU
TDDIWFi5OiC9yN+vtmxZ1RIwawQb2dcHRK8k8aXsXrCnvRwumwnqI/LWoL67WGlO
kZNbyVKE4dh903tFbtLlwsGiMLgZogJB0Ft+zcjEnkDWSA+OkhbOqks/WUMoFlms
yPGbn/35UWRVVHfhL7zFVIKD0m1wZluwOqaVWgJBTsJeHirRbI4RSLnXdsp+T3ly
uDUbXKXjseEtQF4S6eST+c4ckpYgpQtJQPVqYmFShvkk+wKGjYQNXyQZzBUmPERC
H+YF4++g9TcaGw==
-----END CERTIFICATE-----
EOF_CA

# 2. Write Client certificate
cat << 'EOF_CERT' > /etc/swanctl/x509/client.crt
-----BEGIN CERTIFICATE-----
MIIEvTCCAqWgAwIBAgIRAKwK4z84O0gZfXxymtYYDz4wDQYJKoZIhvcNAQELBQAw
gYIxCzAJBgNVBAYTAlVTMRMwEQYDVQQIEwpDYWxpZm9ybmlhMRYwFAYDVQQHEw1T
YW4gRnJhbmNpc2NvMSEwHwYDVQQKExhTd2FuUGFuZWwgUm9vdCBBdXRob3JpdHkx
IzAhBgNVBAMTGlN3YW5QYW5lbCBJbnRlcm5hbCBSb290IENBMB4XDTI2MTExMTAy
MDc1OFoXDTI4MTExMTAyMTc1OFowOTEZMBcGA1UEChMQU3dhblBhbmVsIENsaWVu
dDEcMBoGA1UEAxMTZGF2aWQtaXBob25lLTE2LXBybzCCASIwDQYJKoZIhvcNAQEB
BQADggEPADCCAQoCggEBAON1fVL3MkypePWQ7qE+JYBWRzJoNiUpqQo7Q3ppXSeP
VbEEHcQMYmTSjK6w5eJr6i1ArQvzIzcivMZkntgLkidg3Kp6emKAfj1AqC9QllbT
RTyGxYyM/bxgtsjQkx4mxkgorjNALioQ32ad3wi6GU83Gkmw7ldkO1HW1u6Cw05M
5NZNHS67QyvG9A8UrlgRzu2nu2775NAMApn60LBqVunhIdKjdpG0IKZZjudZisgp
pCzg8tM3kAXxbS137HMO5BMaBrzoTrk8FHPzbExtwtixOkYUpMpHhTPcUmEYR4Kx
g4CJY36BReSO6pguU5xjTaZPzuskCO9iNXtBfAlPC4ECAwEAAaN2MHQwDgYDVR0P
AQH/BAQDAgWgMBMGA1UdJQQMMAoGCCsGAQUFBwMCMAwGA1UdEwEB/wQCMAAwHwYD
VR0jBBgwFoAU9l8lbCbbrD5RsMDinPVnPNIjN68wHgYDVR0RBBcwFYITZGF2aWQt
aXBob25lLTE2LXBybzANBgkqhkiG9w0BAQsFAAOCAgEAYhBvtVCtLXChZgj/w+AD
x4QwLf6O8NLunlLyA1FjwAheLUjWlaRicpDTq/0XDWTn5EH9CgNxMpT4ewdp7pjl
nptlgu/H8NwGuJJPXcBMCnBdtNr0JR9lLRrRvVexee0EzD5lJFWVzNUo53aS6Zyn
6jsyGNFD+x39gaesTqb1IOtpsPmApVXSZiG0o99AoFsYoPg7uwTKEy/2pbzeN43K
eN8FlhwGYPVHWqrOcuTNNMcYOe8BynEJQeat1qc6KPrK59cRxfd2/oJP88AwYl2I
2fABRQEU/pUoBpEFE5W/MH5+m4cx7cL9EJLHxaQ6JJFINYlOzGaTqKebX6dTDkT5
yZPWw2cN4S5/qB/9QBjrX6KkwxMVij3W9i/ELbmDCnZh5LkyIdj/rSSQjw6hbdS/
xJvUUxG7C3ZFkx8nFFVp80X0SKogh2w4t95VGs2tTrlaIJryOmV1RT4SCpMWoJL4
WO08EgvCo4HAiX4nhVKXJbVJwRE8QLiAJy7bfG8jpqBcxSFem1A5zEhYlArJck3K
wPtcAW8ROj5QyBagA5egQR7v23hLKwnIGGGmIZ6BZQGLhBmHN1BcDRH5v1uVOU2o
M+UNcxsLBn1MgL7ERwxke5/FsHX1O4PNoWXOpI6Cu9OQEcEf23Ul/BqZbit6R29B
35rNk9kvVlBXirYRDCb1WM8=
-----END CERTIFICATE-----
EOF_CERT

# 3. Write Client private key
cat << 'EOF_KEY' > /etc/swanctl/rsa/client.key
-----BEGIN RSA PRIVATE KEY-----
MIIEpAIBAAKCAQEA43V9UvcyTKl49ZDuoT4lgFZHMmg2JSmpCjtDemldJ49VsQQd
xAxiZNKMrrDl4mvqLUCtC/MjNyK8xmSe2AuSJ2Dcqnp6YoB+PUCoL1CWVtNFPIbF
jIz9vGC2yNCTHibGSCiuM0AuKhDfZp3fCLoZTzcaSbDuV2Q7UdbW7oLDTkzk1k0d
LrtDK8b0DxSuWBHO7ae7bvvk0AwCmfrQsGpW6eEh0qN2kbQgplmO51mKyCmkLODy
0zeQBfFtLXfscw7kExoGvOhOuTwUc/NsTG3C2LE6RhSkykeFM9xSYRhHgrGDgIlj
foFF5I7qmC5TnGNNpk/O6yQI72I1e0F8CU8LgQIDAQABAoIBAAj6ZWYlyWT67YIt
OdYV02TI7bTWJd5P/NL2uuJKkA7p2p/gQeKFUJ7v1qC7QnUDqHHSfeIdCJSlDjQl
2u/mflrdORM7lA1uSqKqpNjVQqc5CKBr0MFhpqRNtvlJ46dgMBm/YqeK5CeXPzzY
L2pd73YlKShOemHrBr0OELFyajjIFNUfAMj7bXc/5LK57Wk8UiTKZQ2fQKTI8fEW
9/WrYxgviR3H+C/Mhs2AG6mrrgfBZBsKf/AHaG5PGUQRuviatBUdZNhOzO4R4m1a
itroIwBQxfnHcXUtHvIa57wlkKSjsUwRl3UwvAL/mpebIRJSOr3l7BEHyBdgme52
/c4gL5cCgYEA9HXDG32WIC+nz7/Vl3lirDFp9/fUG916Q21b0rjT0GYGb4r4fCPb
MuaxtinDEd/UKDzp7rjl6e3jbA0eyJpvL1F38zbMgZMYPJVTLC3J2dbvOIzi07Wj
+nLN54ICqd4TRclXK3D01gntusF2IiNsOA3F+edlbUWPEpuaX7FKwV8CgYEA7jJG
GDDihf749sIlbYuIUj4s7B73RFV1DuKjihnky5mt3pFBMLdwIjZz5D39r98KCfBm
WeaPbAfnhFnL4h5K9YMwotGA95muStc5KQAccHmbg7Bxxc21Mdz+uzDlm1BNOtpS
i01YEM9vdzP8hqKEJkIpJib5TCGRKdxHeDPc/x8CgYEA4stSLHHx0oCD3mBqB3Kt
EXVyi4PXu9ZY9K9DbhbdI81TkS5L49fcJ7gRDSBngnC2wnHo0fmmC8YQpJDzi0aD
elHJx9GCH4BZDvO2Sxlb2Abovv0t/x3YOKVD09ExKWkHVko53eqYqu5hKE8NixRz
9PkKQBPmbF8JIklMChVQdl8CgYAg/K1gZES6sdjgs3pjdVNnsFygRpmPyRXw/coQ
Mg3RdHLzw8H3cTACTPA3x63+UTEet0A1awyptV6Gobjuk6tl3Mk7gzANGiddDflI
iE9n9C9pSWI4NojFYwmT+u2RWMmZQOr1h7yOd2vEYlqyeoc6lScFtZgIxw1Hcs4j
A2uzmQKBgQC7Nof+QTMeatdrdYwSOUu3mUB+ivciVed46IXaVkrtAXolQqcHWlLR
yj4Wd0VlL+o6cC+MQA1OFIYWDLLYdjJiq/SKccTGcRCTGRH8cA0WRJyEfgWM2nW8
EtoQstogteqTGH6Sv7l1vptyqjEdlKHSoNxVvQsZiomFJld93qKETw==
-----END RSA PRIVATE KEY-----
EOF_KEY
chmod 600 /etc/swanctl/rsa/client.key

# 4. Write swanctl configuration
cat << 'EOF_CONF' > /etc/swanctl/conf.d/swanpanel.conf
connections {
    swanpanel {
        remote_addrs = swan-panel
        vips = 0.0.0.0

        local {
            auth = pubkey
            certs = client.crt
            id = david-iphone-16-pro
        }

        remote {
            auth = pubkey
            id = %any
            cacerts = swanpanel-ca.crt
        }

        children {
            swanpanel-traffic {
                remote_ts = 0.0.0.0/0
                start_action = start
                dpd_action = restart
                close_action = none
                mode = tunnel
                esp_proposals = aes256gcm16-ecp384,aes256-sha256
            }
        }

        version = 2
        mobike = yes
        encap = yes
        proposals = aes256gcm16-prfsha384-ecp384,aes256-sha256-modp2048
    }
}
EOF_CONF

echo "[+] Credentials and configuration installed to /etc/swanctl/."

# 5. Reload credentials and initiate connection if strongSwan is running
if command -v swanctl >/dev/null 2>&1; then
    swanctl --load-all || true
    echo "[*] Connecting to SwanPanel Gateway at swan-panel..."
    swanctl --initiate --child swanpanel-traffic || true
    echo "[+] Done! Check status with: swanctl --list-sas"
else
    echo "[!] strongSwan swanctl not found. Please install strongswan and run: swanctl --load-all"
fi
