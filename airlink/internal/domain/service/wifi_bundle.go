package service

import (
	"archive/zip"
	"bytes"
	"crypto/sha1"
	"crypto/x509"
	_ "embed"
	"encoding/pem"
	"fmt"
	"strings"
)

//go:embed assets/binawan-wifi-ca.pem
var wifiCAPEM []byte

type WiFiBundle struct {
	Filename string
	Data     []byte
}

// SupportedWiFiPlatform separates the old raw-P12 download from platform packages.
func SupportedWiFiPlatform(platform string) bool {
	switch platform {
	case "certificate", "windows", "macos", "ios", "android", "linux":
		return true
	default:
		return false
	}
}

// BuildWiFiBundle packages the selected user's certificate with the public CA and
// platform-specific setup files. The P12 password is deliberately not put in ZIP.
func BuildWiFiBundle(platform, username, device string, p12 []byte) (*WiFiBundle, error) {
	if !SupportedWiFiPlatform(platform) || platform == "certificate" {
		return nil, fmt.Errorf("platform paket tidak didukung")
	}
	block, _ := pem.Decode(wifiCAPEM)
	if block == nil || block.Type != "CERTIFICATE" {
		return nil, fmt.Errorf("CA WiFi tidak valid")
	}
	ca, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse CA WiFi: %w", err)
	}
	if len(p12) == 0 {
		return nil, fmt.Errorf("sertifikat pengguna kosong")
	}

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	add := func(name string, data []byte) error {
		file, err := writer.Create(name)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		return err
	}
	files := []struct {
		name string
		data []byte
	}{
		{"client.p12", p12},
		{"Binawan-WiFi-CA.pem", wifiCAPEM},
		{"README.txt", []byte(bundleReadme(platform, username, device))},
	}
	if platform == "windows" {
		hash := sha1.Sum(ca.Raw)
		thumbprint := strings.ToUpper(fmt.Sprintf("% X", hash[:]))
		files = append(files,
			struct {
				name string
				data []byte
			}{"Binawan-WiFi-CA.cer", ca.Raw},
			struct {
				name string
				data []byte
			}{"wifi-BinawanStaff.xml", []byte(fmt.Sprintf(windowsWiFiXML, thumbprint))},
			struct {
				name string
				data []byte
			}{"install-wifi.bat", []byte(strings.ReplaceAll(windowsBAT, "\n", "\r\n"))},
			struct {
				name string
				data []byte
			}{"install-wifi.ps1", []byte(windowsPowerShell)},
		)
	}
	for _, file := range files {
		if err := add(file.name, file.data); err != nil {
			_ = writer.Close()
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &WiFiBundle{
		Filename: safeBundlePart(username) + "-" + safeBundlePart(device) + "-" + platform + ".zip",
		Data:     buffer.Bytes(),
	}, nil
}

func safeBundlePart(input string) string {
	return strings.Map(func(char rune) rune {
		if char >= 'a' && char <= 'z' || char >= 'A' && char <= 'Z' || char >= '0' && char <= '9' || char == '-' || char == '_' {
			return char
		}
		return '_'
	}, input)
}

func bundleReadme(platform, username, device string) string {
	intro := fmt.Sprintf("Binawan_Staff - paket WiFi %s\r\nPengguna: %s\r\nPerangkat: %s\r\n\r\n", platform, username, device)
	common := "Paket ini berisi identitas pribadi. Jangan dibagikan.\r\nPassword client.p12 ditampilkan terpisah di portal Airlink setelah unduh.\r\nJika Windows meminta username/password WiFi, profil EAP-TLS belum berhasil dipasang.\r\n"
	switch platform {
	case "windows":
		return intro + "1. Ekstrak SELURUH isi ZIP ke satu folder.\r\n2. Double-click install-wifi.bat sebagai user Windows yang akan memakai WiFi.\r\n3. Masukkan password P12 dari portal (bukan username/password WiFi).\r\n4. Installer memasang CA, identitas, dan mengganti profil Binawan_Staff lama dengan EAP-TLS.\r\n5. Sambungkan ke Binawan_Staff. Jika gagal, baca error di jendela installer.\r\n\r\n" + common
	case "macos":
		return intro + "1. Impor Binawan-WiFi-CA.pem ke Keychain dan set trust sesuai kebijakan IT.\r\n2. Impor client.p12 ke login Keychain memakai password dari portal.\r\n3. Hubungkan Binawan_Staff dengan EAP-TLS/certificate; validasi server radius.binawan.ac.id.\r\n\r\n" + common
	case "ios":
		return intro + "1. Buka CA dan client.p12 di iPhone/iPad, lalu pasang lewat Settings > Profile Downloaded.\r\n2. Masukkan password P12 dari portal. Aktifkan trust CA sesuai instruksi IT.\r\n3. Hubungkan Binawan_Staff memakai sertifikat pengguna (EAP-TLS).\r\n\r\n" + common
	case "android":
		return intro + "1. Pasang CA dan client.p12 dari Settings > Security > Install certificate.\r\n2. Masukkan password P12 dari portal.\r\n3. Pada Binawan_Staff pilih EAP method TLS, CA Binawan, sertifikat pengguna ini, dan domain radius.binawan.ac.id.\r\n\r\n" + common
	default: // linux
		return intro + "1. Impor client.p12 dan CA ke NetworkManager/sertifikat sistem.\r\n2. Atur Binawan_Staff: WPA2-Enterprise, EAP-TLS, CA Binawan, client.p12 dan password dari portal.\r\n3. Validasi nama server radius.binawan.ac.id.\r\n\r\n" + common
	}
}

// This follows Microsoft's EAP-TLS WLAN profile structure (Eap/Type/EapType),
// unlike the earlier profile which placed EapTls directly below Config.
const windowsWiFiXML = `<?xml version="1.0" encoding="utf-8"?>
<WLANProfile xmlns="http://www.microsoft.com/networking/WLAN/profile/v1">
  <name>Binawan_Staff</name>
  <SSIDConfig><SSID><name>Binawan_Staff</name></SSID></SSIDConfig>
  <connectionType>ESS</connectionType>
  <connectionMode>auto</connectionMode>
  <MSM><security>
    <authEncryption><authentication>WPA2</authentication><encryption>AES</encryption><useOneX>true</useOneX></authEncryption>
    <OneX xmlns="http://www.microsoft.com/networking/OneX/v1">
      <authMode>user</authMode>
      <EAPConfig><EapHostConfig xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
        <EapMethod>
          <Type xmlns="http://www.microsoft.com/provisioning/EapCommon">13</Type>
          <VendorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorId>
          <VendorType xmlns="http://www.microsoft.com/provisioning/EapCommon">0</VendorType>
          <AuthorId xmlns="http://www.microsoft.com/provisioning/EapCommon">0</AuthorId>
        </EapMethod>
        <Config xmlns="http://www.microsoft.com/provisioning/EapHostConfig">
          <Eap xmlns="http://www.microsoft.com/provisioning/BaseEapConnectionPropertiesV1">
            <Type>13</Type>
            <EapType xmlns="http://www.microsoft.com/provisioning/EapTlsConnectionPropertiesV1">
              <CredentialsSource><CertificateStore><SimpleCertSelection>true</SimpleCertSelection></CertificateStore></CredentialsSource>
              <ServerValidation>
                <DisableUserPromptForServerValidation>true</DisableUserPromptForServerValidation>
                <ServerNames>radius.binawan.ac.id</ServerNames>
                <TrustedRootCA>%s</TrustedRootCA>
              </ServerValidation>
              <DifferentUsername>false</DifferentUsername>
              <PerformServerValidation xmlns="http://www.microsoft.com/provisioning/EapTlsConnectionPropertiesV2">true</PerformServerValidation>
              <AcceptServerName xmlns="http://www.microsoft.com/provisioning/EapTlsConnectionPropertiesV2">true</AcceptServerName>
            </EapType>
          </Eap>
        </Config>
      </EapHostConfig></EAPConfig>
    </OneX>
  </security></MSM>
</WLANProfile>
`

const windowsBAT = `@echo off
setlocal
title Setup WiFi Binawan_Staff (EAP-TLS)
cd /d "%~dp0"
powershell.exe -NoProfile -ExecutionPolicy Bypass -File "%~dp0install-wifi.ps1"
set "RESULT=%ERRORLEVEL%"
if not "%RESULT%"=="0" echo Installer gagal. Baca pesan error di atas; jangan masukkan username/password WiFi.
pause
exit /b %RESULT%
`

const windowsPowerShell = `$ErrorActionPreference = 'Stop'
try {
    $folder = Split-Path -Parent $MyInvocation.MyCommand.Path
    $pfx = Join-Path $folder 'client.p12'
    if (-not (Test-Path -LiteralPath $pfx)) {
        $candidates = @(Get-ChildItem -LiteralPath $folder -Filter '*.p12' -File)
        if ($candidates.Count -ne 1) { throw 'Taruh tepat satu file .p12 di folder installer (atau gunakan paket per-user dari portal Airlink).' }
        $pfx = $candidates[0].FullName
    }
    $ca = Join-Path $folder 'Binawan-WiFi-CA.cer'
    $profile = Join-Path $folder 'wifi-BinawanStaff.xml'
    foreach ($file in @($pfx, $ca, $profile)) {
        if (-not (Test-Path -LiteralPath $file)) { throw "File paket tidak lengkap: $file. Ekstrak seluruh ZIP dulu." }
    }

    Write-Host '[1/3] Memasang CA Binawan untuk user Windows ini...'
    Import-Certificate -FilePath $ca -CertStoreLocation 'Cert:\CurrentUser\Root' | Out-Null

    Write-Host '[2/3] Memasang sertifikat pribadi...'
    $password = Read-Host 'Masukkan password P12 dari portal Airlink' -AsSecureString
    $certs = @(Import-PfxCertificate -FilePath $pfx -CertStoreLocation 'Cert:\CurrentUser\My' -Password $password -Exportable:$false)
    if (-not ($certs | Where-Object { $_.HasPrivateKey })) { throw 'P12 tidak menghasilkan sertifikat dengan private key.' }

    Write-Host '[3/3] Mengganti profil WiFi lama dengan EAP-TLS...'
    & netsh.exe wlan show profile name='Binawan_Staff' *> $null
    if ($LASTEXITCODE -eq 0) {
        & netsh.exe wlan delete profile name='Binawan_Staff'
        if ($LASTEXITCODE -ne 0) { throw 'Profil lama tidak dapat dihapus. Minta IT menghapus profil Binawan_Staff lama, lalu jalankan lagi.' }
    }
    & netsh.exe wlan add profile "filename=$profile" user=current
    if ($LASTEXITCODE -ne 0) { throw 'Windows menolak profil EAP-TLS. Catat pesan netsh di atas untuk IT.' }

    Write-Host 'Selesai. Sambungkan ke Binawan_Staff tanpa username/password WiFi.' -ForegroundColor Green
    exit 0
} catch {
    Write-Host "GAGAL: $($_.Exception.Message)" -ForegroundColor Red
    exit 1
}
`
