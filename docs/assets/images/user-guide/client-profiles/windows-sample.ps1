# SwanPanel Windows Native IKEv2 VPN Setup Script
# Run this script with Administrator privileges in PowerShell

$ErrorActionPreference = "Stop"
Write-Host "[SwanPanel] Installing native IKEv2 VPN configuration for david-iphone-16-pro..." -ForegroundColor Cyan

# 1. Save and import Root CA
$caCertPEM = @"
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
"@
$caCertPath = "$env:TEMP\swanpanel-ca.crt"
Set-Content -Path $caCertPath -Value $caCertPEM
Import-Certificate -FilePath $caCertPath -CertStoreLocation Cert:\LocalMachine\Root | Out-Null
Remove-Item -Path $caCertPath -Force
Write-Host "[SwanPanel] Root CA installed into LocalMachine\Root store." -ForegroundColor Green

# 2. Add or update native IKEv2 VPN Connection
$vpnName = "SwanPanel VPN"
$existingVpn = Get-VpnConnection -Name $vpnName -ErrorAction SilentlyContinue

if ($existingVpn) {
    Write-Host "[SwanPanel] Updating existing VPN connection '$vpnName'..." -ForegroundColor Yellow
    Set-VpnConnection -Name $vpnName -ServerAddress "swan-panel" -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -EncryptionLevel Required -SplitTunneling $true
} else {
    Write-Host "[SwanPanel] Creating new native IKEv2 VPN connection '$vpnName'..." -ForegroundColor Green
    Add-VpnConnection -Name $vpnName -ServerAddress "swan-panel" -TunnelType Ikev2 -AuthenticationMethod MachineCertificate -EncryptionLevel Required -SplitTunneling $true
}

# 3. Configure robust IPsec cryptography for Windows
Set-VpnConnectionIPsecConfiguration -ConnectionName $vpnName -AuthenticationTransformConstants GCMAES256 -CipherTransformConstants GCMAES256 -EncryptionMethod GCMAES256 -IntegrityCheckMethod SHA384 -DHGroup Group14 -PfsGroup PFS2048 -Force

Write-Host "[SwanPanel] Setup completed successfully! You can now connect from Windows Settings > Network > VPN." -ForegroundColor Cyan
