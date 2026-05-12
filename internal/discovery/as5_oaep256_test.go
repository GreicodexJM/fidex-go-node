package discovery

import (
	"encoding/json"
	"testing"
)

// TestGenerateAS5Config_AdvertisesDualEncryption guards the on-wire shape
// produced by GenerateAS5Config for FID-4: both the legacy single field
// AND the new supported_encryption_algorithms array must be present.
// Legacy peers reading only encryption_algorithm see "RSA-OAEP" (no
// behaviour change); FID-4-aware peers can negotiate to RSA-OAEP-256.
func TestGenerateAS5Config_AdvertisesDualEncryption(t *testing.T) {
	cfg := GenerateAS5Config(NodeConfig{
		NodeID:           "urn:gln:fid4-test",
		OrganizationName: "FID-4 Test Node",
		BaseURL:          "https://node.example.com",
		PublicDomain:     "node.example.com",
	})

	if cfg.Security.EncryptionAlgorithm != "RSA-OAEP" {
		t.Errorf("legacy single field encryption_algorithm must stay 'RSA-OAEP' for back-compat, got %q",
			cfg.Security.EncryptionAlgorithm)
	}

	got := cfg.Security.SupportedEncryptionAlgorithms
	if len(got) != 2 {
		t.Fatalf("expected supported_encryption_algorithms with 2 entries, got %d: %v", len(got), got)
	}

	want := map[string]bool{"RSA-OAEP": false, "RSA-OAEP-256": false}
	for _, alg := range got {
		if _, ok := want[alg]; !ok {
			t.Errorf("unexpected algorithm %q in advertisement", alg)
			continue
		}
		want[alg] = true
	}
	for alg, seen := range want {
		if !seen {
			t.Errorf("AS5 advertisement missing required algorithm %q", alg)
		}
	}
}

// TestGenerateAS5Config_JSONShape pins the JSON wire-level field name so
// peers know exactly what key to read. Catches a future accidental rename
// (e.g. snake_case → camelCase) that would silently break interop.
func TestGenerateAS5Config_JSONShape(t *testing.T) {
	cfg := GenerateAS5Config(NodeConfig{
		NodeID:           "urn:gln:fid4-json",
		OrganizationName: "FID-4 JSON Test",
		BaseURL:          "https://node.example.com",
	})

	raw, err := json.Marshal(cfg.Security)
	if err != nil {
		t.Fatalf("marshal Security: %v", err)
	}
	body := string(raw)

	mustContain := []string{
		`"encryption_algorithm":"RSA-OAEP"`,
		`"supported_encryption_algorithms":["RSA-OAEP","RSA-OAEP-256"]`,
		`"signature_algorithm":"RS256"`,
		`"content_encryption":"A256GCM"`,
	}
	for _, s := range mustContain {
		if !contains(body, s) {
			t.Errorf("Security JSON missing %q.\nfull body: %s", s, body)
		}
	}
}

// TestResolveSupportedEncryptionAlgorithms exercises the back-compat
// resolution rules an AS5 consumer relies on when ingesting a partner's
// discovery document.
func TestResolveSupportedEncryptionAlgorithms(t *testing.T) {
	cases := []struct {
		name string
		cfg  *AS5Configuration
		want []string
	}{
		{
			name: "modern peer with array",
			cfg: &AS5Configuration{
				Security: AS5SecurityConfig{
					EncryptionAlgorithm:           "RSA-OAEP",
					SupportedEncryptionAlgorithms: []string{"RSA-OAEP", "RSA-OAEP-256"},
				},
			},
			want: []string{"RSA-OAEP", "RSA-OAEP-256"},
		},
		{
			name: "legacy peer with single field only",
			cfg: &AS5Configuration{
				Security: AS5SecurityConfig{
					EncryptionAlgorithm: "RSA-OAEP",
				},
			},
			want: []string{"RSA-OAEP"},
		},
		{
			name: "legacy peer advertising only 256 via single field",
			cfg: &AS5Configuration{
				Security: AS5SecurityConfig{
					EncryptionAlgorithm: "RSA-OAEP-256",
				},
			},
			want: []string{"RSA-OAEP-256"},
		},
		{
			name: "completely empty Security",
			cfg:  &AS5Configuration{},
			want: nil,
		},
		{
			name: "nil config",
			cfg:  nil,
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveSupportedEncryptionAlgorithms(tc.cfg)
			if !sliceEq(got, tc.want) {
				t.Errorf("want %v, got %v", tc.want, got)
			}
		})
	}
}

// TestResolveSupportedEncryptionAlgorithms_ReturnsCopy guards that mutating
// the returned slice does not corrupt the source config. Important — the
// negotiator hands this out as the partner's capability list and the
// worker is free to manipulate it.
func TestResolveSupportedEncryptionAlgorithms_ReturnsCopy(t *testing.T) {
	cfg := &AS5Configuration{
		Security: AS5SecurityConfig{
			SupportedEncryptionAlgorithms: []string{"RSA-OAEP", "RSA-OAEP-256"},
		},
	}
	out := ResolveSupportedEncryptionAlgorithms(cfg)
	out[0] = "MUTATED"
	if cfg.Security.SupportedEncryptionAlgorithms[0] != "RSA-OAEP" {
		t.Errorf("source slice mutated through returned reference: %v",
			cfg.Security.SupportedEncryptionAlgorithms)
	}
}

func contains(haystack, needle string) bool {
	return indexOf(haystack, needle) >= 0
}

func indexOf(s, sub string) int {
outer:
	for i := 0; i+len(sub) <= len(s); i++ {
		for j := 0; j < len(sub); j++ {
			if s[i+j] != sub[j] {
				continue outer
			}
		}
		return i
	}
	return -1
}

func sliceEq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
