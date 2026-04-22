package handler

import "testing"

func TestContractConfig_Validate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ContractConfig
		wantErr bool
	}{
		{
			name: "valid",
			cfg: ContractConfig{
				Name:    "usdt",
				Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7",
				Events:  []string{"Transfer(address,address,uint256)"},
			},
			wantErr: false,
		},
		{
			name:    "empty name",
			cfg:     ContractConfig{Name: "", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Events: []string{"X()"}},
			wantErr: true,
		},
		{
			name:    "empty address",
			cfg:     ContractConfig{Name: "x", Address: "", Events: []string{"X()"}},
			wantErr: true,
		},
		{
			name:    "malformed address (too short)",
			cfg:     ContractConfig{Name: "x", Address: "0xdead", Events: []string{"X()"}},
			wantErr: true,
		},
		{
			name:    "malformed address (non-hex)",
			cfg:     ContractConfig{Name: "x", Address: "not-an-address", Events: []string{"X()"}},
			wantErr: true,
		},
		{
			name:    "no events",
			cfg:     ContractConfig{Name: "x", Address: "0xdAC17F958D2ee523a2206206994597C13D831ec7", Events: nil},
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() err = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}
