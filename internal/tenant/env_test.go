package tenant

import (
	"testing"
)

func TestLoadEnv(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		want    EnvConfig
		wantErr bool
	}{
		{
			name: "defaults",
			env:  map[string]string{},
			want: EnvConfig{
				ConfigsDir: "configs",
				GRPCPort:   50051,
				Host:       "0.0.0.0",
				RedisAddr:  "localhost:6379",
				RedisDB:    0,
				LogLevel:   "info",
				LogFormat:  "json",
			},
			wantErr: false,
		},
		{
			name: "overrides",
			env: map[string]string{
				"AIBOX_CONFIGS_DIR": "/tmp",
				"AIBOX_GRPC_PORT":   "8080",
				"AIBOX_HOST":        "127.0.0.1",
				"REDIS_ADDR":        "redis:6379",
				"REDIS_DB":          "1",
				"AIBOX_LOG_LEVEL":   "debug",
			},
			want: EnvConfig{
				ConfigsDir: "/tmp",
				GRPCPort:   8080,
				Host:       "127.0.0.1",
				RedisAddr:  "redis:6379",
				RedisDB:    1,
				LogLevel:   "debug",
				LogFormat:  "json",
			},
			wantErr: false,
		},
		{
			name: "invalid port",
			env: map[string]string{
				"AIBOX_GRPC_PORT": "invalid",
			},
			wantErr: true,
		},
		{
			name: "tls missing cert",
			env: map[string]string{
				"AIBOX_TLS_ENABLED": "true",
			},
			wantErr: true,
		},
		{
			name: "tls valid",
			env: map[string]string{
				"AIBOX_TLS_ENABLED":   "true",
				"AIBOX_TLS_CERT_FILE": "cert.pem",
				"AIBOX_TLS_KEY_FILE":  "key.pem",
			},
			want: EnvConfig{
				ConfigsDir:  "configs",
				GRPCPort:    50051,
				Host:        "0.0.0.0",
				RedisAddr:   "localhost:6379",
				TLSEnabled:  true,
				TLSCertFile: "cert.pem",
				TLSKeyFile:  "key.pem",
				LogLevel:    "info",
				LogFormat:   "json",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			got, err := loadEnv()
			if (err != nil) != tt.wantErr {
				t.Errorf("loadEnv() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// Compare relevant fields
				if got.ConfigsDir != tt.want.ConfigsDir {
					t.Errorf("ConfigsDir = %v, want %v", got.ConfigsDir, tt.want.ConfigsDir)
				}
				if got.GRPCPort != tt.want.GRPCPort {
					t.Errorf("GRPCPort = %v, want %v", got.GRPCPort, tt.want.GRPCPort)
				}
				if got.TLSEnabled != tt.want.TLSEnabled {
					t.Errorf("TLSEnabled = %v, want %v", got.TLSEnabled, tt.want.TLSEnabled)
				}
			}
		})
	}
}
